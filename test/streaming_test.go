package e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamingSSEPassthrough(t *testing.T) {
	sseBody := loadFixture(t, "chat.stream.sse")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			_, _ = w.Write(sseBody)
			flusher.Flush()
			return
		}
		_, _ = w.Write(sseBody)
	}))
	t.Cleanup(up.Close)

	h := newHarness(t, harnessOpts{
		guardEnabled:  true,
		mgmtEnabled:   true,
		budgetLimit:   10_000_000,
		openaiURL:     up.URL,
		anthropicURL:  up.URL,
		openrouterURL: up.URL,
	})

	status, body, hdr := h.do(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	if !strings.Contains(hdr.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("Content-Type = %q", hdr.Get("Content-Type"))
	}
	if string(body) != string(sseBody) {
		t.Fatalf("SSE body mutated:\n got=%q\nwant=%q", body, sseBody)
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "completed" {
		t.Fatalf("usage=%q", ev.Status)
	}
}

func TestOpenRouterUsageCostPreferred(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(loadFixture(t, "openrouter.cost.json"))
	}))
	t.Cleanup(up.Close)

	h := newHarness(t, harnessOpts{
		guardEnabled:  true,
		mgmtEnabled:   true,
		budgetLimit:   10_000_000,
		openaiURL:     up.URL,
		openrouterURL: up.URL,
		anthropicURL:  up.URL,
	})

	status, _, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(map[string]string{
		"X-TokenGuard-Provider": "openrouter",
	}))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "completed" {
		t.Fatalf("usage=%q", ev.Status)
	}
	if ev.ActualCostMicroUSD != 42 {
		t.Fatalf("ActualCostMicroUSD = %d, want 42 (from usage.cost)", ev.ActualCostMicroUSD)
	}
}

func TestNonStreamUsageTokens(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, budgetLimit: 10_000_000})
	status, body, _ := h.do(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	want := string(loadFixture(t, "chat.response.json"))
	if strings.TrimSpace(string(body)) != strings.TrimSpace(want) {
		t.Fatalf("body mismatch")
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.InputTokens != 12 || ev.OutputTokens != 3 {
		t.Fatalf("tokens in/out = %d/%d, want 12/3", ev.InputTokens, ev.OutputTokens)
	}
	if ev.ActualCostMicroUSD <= 0 {
		t.Fatalf("expected positive actual cost, got %d", ev.ActualCostMicroUSD)
	}
}
