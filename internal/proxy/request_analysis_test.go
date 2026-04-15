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
