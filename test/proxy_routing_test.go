package e2e

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestProxyRoutingMultiProvider(t *testing.T) {
	var (
		mu       sync.Mutex
		lastPath string
		lastAuth string
		sawTG    bool
		hostHit  string
	)
	openaiUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		lastAuth = r.Header.Get("Authorization")
		sawTG = r.Header.Get("X-TokenGuard-API-Key") != "" || r.Header.Get("X-TokenGuard-Provider") != ""
		hostHit = "openai"
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "chat.response.json"))
	}))
	t.Cleanup(openaiUp.Close)

	anthropicUp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastPath = r.URL.Path
		hostHit = "anthropic"
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "anthropic.response.json"))
	}))
	t.Cleanup(anthropicUp.Close)

	h := newHarness(t, harnessOpts{
		guardEnabled:  true,
		mgmtEnabled:   true,
		budgetLimit:   10_000_000,
		openaiURL:     openaiUp.URL,
		anthropicURL:  anthropicUp.URL,
		openrouterURL: openaiUp.URL,
	})

	status, _, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusOK {
		t.Fatalf("openai chat status=%d", status)
	}
	_ = h.waitUsage(2 * time.Second)
	mu.Lock()
	if lastPath != "/v1/chat/completions" {
		t.Fatalf("path = %q", lastPath)
	}
	if lastAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", lastAuth)
	}
	if sawTG {
		t.Fatal("TokenGuard headers leaked to upstream")
	}
	mu.Unlock()

	status, _, _ = h.doJSON(http.MethodPost, "/v1/messages", loadFixture(t, "anthropic.request.json"), h.proxyHeaders(map[string]string{
		"X-TokenGuard-Provider": "anthropic",
	}))
	if status != http.StatusOK {
		t.Fatalf("anthropic status=%d", status)
	}
	_ = h.waitUsage(2 * time.Second)
	mu.Lock()
	if hostHit != "anthropic" {
		t.Fatalf("expected anthropic upstream, hostHit=%q", hostHit)
	}
	mu.Unlock()

	status, _, _ = h.doJSON(http.MethodPost, "/v1/messages", loadFixture(t, "anthropic.request.json"), map[string]string{
		"Content-Type":         "application/json",
		"Authorization":        "Bearer sk-test",
		"X-TokenGuard-API-Key": h.apiKey,
	})
	if status != http.StatusOK {
		t.Fatalf("inferred anthropic status=%d", status)
	}
	_ = h.waitUsage(2 * time.Second)

	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(map[string]string{
		"X-TokenGuard-Provider": "not-a-provider",
	}))
	if status != http.StatusBadRequest {
		t.Fatalf("unknown provider status=%d data=%v", status, data)
	}
}

func TestProxyOpenRouterPathJoin(t *testing.T) {
	var gotPath string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "chat.response.json"))
	}))
	t.Cleanup(up.Close)

	h := newHarness(t, harnessOpts{
		guardEnabled:  true,
		mgmtEnabled:   true,
		budgetLimit:   10_000_000,
		openrouterURL: up.URL,
		openaiURL:     up.URL,
		anthropicURL:  up.URL,
	})
	status, _, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(map[string]string{
		"X-TokenGuard-Provider": "openrouter",
	}))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	_ = h.waitUsage(2 * time.Second)
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("path = %q, want /v1/chat/completions (no doubled /v1)", gotPath)
	}
}
