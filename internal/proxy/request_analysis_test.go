package proxy

import (
	"net/http"
	"strings"
	"testing"
)

func TestAnalyzeRequestExtractsBudgetFields(t *testing.T) {
	body := []byte(`{
	  "model":"gpt-test",
	  "max_tokens":7,
	  "metadata":{"session_id":"session-from-body"},
	  "messages":[{"role":"user","content":"hello"},{"role":"assistant","content":[{"type":"text","text":"world"}]}]
	}`)
	req, err := http.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set(tokenGuardSessionHeader, "session-from-header")

	analysis, err := analyzeRequest(req, body, fakeTokenEncoder{}, 4096)
	if err != nil {
		t.Fatalf("analyzeRequest returned error: %v", err)
	}
	if analysis.Model != "gpt-test" {
		t.Fatalf("Model = %q, want gpt-test", analysis.Model)
	}
	if analysis.MaxOutputTokens != 7 {
		t.Fatalf("MaxOutputTokens = %d, want 7", analysis.MaxOutputTokens)
	}
	if analysis.SessionID != "session-from-header" {
		t.Fatalf("SessionID = %q, want header session", analysis.SessionID)
	}
	if analysis.InputTokens <= 0 {
		t.Fatalf("InputTokens = %d, want positive count", analysis.InputTokens)
	}
	if strings.Contains(string(analysis.SemanticPayload), "session-from-body") {
		t.Fatalf("semantic payload leaked metadata: %s", string(analysis.SemanticPayload))
	}
}

func TestAnalyzeRequestUsesDefaultMaxOutputTokens(t *testing.T) {
	body := []byte(`{"model":"gpt-test","prompt":"hello"}`)
	req, err := http.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(string(body)))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}

	analysis, err := analyzeRequest(req, body, fakeTokenEncoder{}, 123)
	if err != nil {
		t.Fatalf("analyzeRequest returned error: %v", err)
	}
	if analysis.MaxOutputTokens != 123 {
		t.Fatalf("MaxOutputTokens = %d, want default", analysis.MaxOutputTokens)
	}
}

func TestApplyCombinedBearerAcceptsAnthropicAPIKey(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/v1/messages", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set("x-api-key", "tg_test:sk-ant-third")
	applyCombinedBearer(req)
	if got := tokenGuardAPIKey(req); got != "tg_test" {
		t.Fatalf("tokenGuardAPIKey = %q", got)
	}
	if got := req.Header.Get("x-api-key"); got != "sk-ant-third" {
		t.Fatalf("x-api-key = %q, want provider key only", got)
	}
	if got := req.Header.Get(tokenGuardSessionHeader); got != cursorSessionID("tg_test") {
		t.Fatalf("session = %q", got)
	}
}

func TestSplitCombinedBearer(t *testing.T) {
	tests := []struct {
		header      string
		tgKey       string
		providerKey string
		ok          bool
	}{
		{header: "Bearer tg_test:sk-third", tgKey: "tg_test", providerKey: "sk-third", ok: true},
		{header: "bearer tg_abc:sk-ant:extra", tgKey: "tg_abc", providerKey: "sk-ant:extra", ok: true},
		{header: "Bearer sk-only"},
		{header: "Bearer tg_nocolon"},
		{header: "Bearer tg_:sk-third"},
		{header: "Bearer tg_test:"},
	}
	for _, tt := range tests {
		tgKey, providerKey, ok := splitCombinedBearer(tt.header)
		if ok != tt.ok || tgKey != tt.tgKey || providerKey != tt.providerKey {
			t.Fatalf("splitCombinedBearer(%q) = %q, %q, %v", tt.header, tgKey, providerKey, ok)
		}
	}
}

func TestTokenGuardAPIKeySupportsPrimaryAndFallbackHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "/", nil)
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set(tokenGuardAPIKeyAltHeader, "fallback")
	if got := tokenGuardAPIKey(req); got != "fallback" {
		t.Fatalf("tokenGuardAPIKey = %q, want fallback", got)
	}
	req.Header.Set(tokenGuardAPIKeyHeader, "primary")
	if got := tokenGuardAPIKey(req); got != "primary" {
		t.Fatalf("tokenGuardAPIKey = %q, want primary", got)
	}
}
