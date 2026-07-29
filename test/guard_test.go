package e2e

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGuardMissingAndInvalidKey(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, budgetLimit: 10_000_000})

	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer sk-test",
	})
	if status != http.StatusUnauthorized || data["code"] != "missing_api_key" {
		t.Fatalf("missing key: status=%d data=%v", status, data)
	}

	status, data, _ = h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), map[string]string{
		"Content-Type":         "application/json",
		"Authorization":        "Bearer sk-test",
		"X-TokenGuard-API-Key": "tg_not_real",
	})
	if status != http.StatusUnauthorized || data["code"] != "invalid_api_key" {
		t.Fatalf("invalid key: status=%d data=%v", status, data)
	}
}

func TestGuardUnknownModel(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, budgetLimit: 10_000_000})
	req := []byte(`{"model":"totally-unknown-model","messages":[{"role":"user","content":"x"}],"max_tokens":8}`)
	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", req, h.proxyHeaders(nil))
	if status != http.StatusBadRequest || data["code"] != "pricing_not_configured" {
		t.Fatalf("status=%d data=%v", status, data)
	}
}

func TestGuardRequestTooLarge(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, maxRequestBytes: 64, budgetLimit: 10_000_000})
	big := []byte(`{"model":"gpt-e2e","messages":[{"role":"user","content":"` + strings.Repeat("x", 200) + `"}],"max_tokens":8}`)
	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", big, h.proxyHeaders(nil))
	if status != http.StatusRequestEntityTooLarge || data["code"] != "request_too_large" {
		t.Fatalf("status=%d data=%v", status, data)
	}
}

func TestGuardBudgetExceeded(t *testing.T) {
	hits := &hitCountingUpstream{
		next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(loadFixture(t, "chat.response.json"))
		}),
	}
	h := newHarness(t, harnessOpts{
		guardEnabled: true,
		mgmtEnabled:  true,
		budgetLimit:  1, // 1 micro-USD — any real estimate exceeds this
		upstream:     hits,
	})

	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusPaymentRequired {
		t.Fatalf("status=%d data=%v", status, data)
	}
	if data["code"] != "budget_exceeded" {
		t.Fatalf("code=%v", data["code"])
	}
	if _, ok := data["available_microusd"]; !ok {
		t.Fatalf("missing available_microusd in %#v", data)
	}
	if _, ok := data["estimated_cost_microusd"]; !ok {
		t.Fatalf("missing estimated_cost_microusd in %#v", data)
	}
	if hits.Hits() != 0 {
		t.Fatalf("upstream hits = %d, want 0 (budget must block before forward)", hits.Hits())
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "blocked_budget" {
		t.Fatalf("usage status=%q", ev.Status)
	}
	h.store.mu.Lock()
	spent := h.store.budgets[h.userID].SpentMicroUSD
	h.store.mu.Unlock()
	if spent != 0 {
		t.Fatalf("spent = %d after budget block, want 0", spent)
	}

	status, _, _ = h.doJSON(http.MethodPatch, "/mgmt/budget", map[string]any{
		"user_id":    h.userID,
		"budget_usd": 10,
	}, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("extend budget status=%d", status)
	}
	status, _, _ = h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusOK {
		t.Fatalf("retry status=%d", status)
	}
	if hits.Hits() != 1 {
		t.Fatalf("upstream hits after extend = %d, want 1", hits.Hits())
	}
	ev = h.waitUsage(2 * time.Second)
	if ev.Status != "completed" {
		t.Fatalf("retry usage=%q", ev.Status)
	}
}

func TestGuardLoopTripsAfterRepeatedIdenticalRequests(t *testing.T) {
	const threshold = 3
	hits := &hitCountingUpstream{
		next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(loadFixture(t, "chat.response.json"))
		}),
	}
	breaker := newCountingBreaker(threshold)
	h := newHarness(t, harnessOpts{
		guardEnabled: true,
		mgmtEnabled:  true,
		budgetLimit:  10_000_000,
		breaker:      breaker,
		upstream:     hits,
	})

	body := loadFixture(t, "chat.request.json")
	hdr := h.proxyHeaders(map[string]string{"X-TokenGuard-Session-ID": "agent-loop-sess"})

	// First (threshold-1) identical requests should pass.
	for i := 0; i < int(threshold)-1; i++ {
		status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", body, hdr)
		if status != http.StatusOK {
			t.Fatalf("request %d status=%d data=%v", i+1, status, data)
		}
		ev := h.waitUsage(2 * time.Second)
		if ev.Status != "completed" {
			t.Fatalf("request %d usage=%q", i+1, ev.Status)
		}
	}
	if hits.Hits() != int(threshold)-1 {
		t.Fatalf("upstream hits before trip = %d, want %d", hits.Hits(), threshold-1)
	}

	// Nth identical payload trips the loop breaker — upstream must not be called.
	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", body, hdr)
	if status != http.StatusConflict || data["code"] != "loop_detected" {
		t.Fatalf("trip status=%d data=%v", status, data)
	}
	if hits.Hits() != int(threshold)-1 {
		t.Fatalf("upstream hits after trip = %d, want still %d", hits.Hits(), threshold-1)
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "blocked_loop" {
		t.Fatalf("usage=%q", ev.Status)
	}

	// Different session should not share the counter.
	status, _, _ = h.doJSON(http.MethodPost, "/v1/chat/completions", body, h.proxyHeaders(map[string]string{
		"X-TokenGuard-Session-ID": "other-session",
	}))
	if status != http.StatusOK {
		t.Fatalf("other session status=%d", status)
	}
	_ = h.waitUsage(2 * time.Second)
	if hits.Hits() != int(threshold) {
		t.Fatalf("upstream hits after other session = %d, want %d", hits.Hits(), threshold)
	}
}

func TestGuardLoopSkippedWithoutSessionID(t *testing.T) {
	// Even a permanently-tripped breaker is skipped when no session id is sent.
	h := newHarness(t, harnessOpts{
		guardEnabled: true,
		mgmtEnabled:  true,
		budgetLimit:  10_000_000,
		breaker:      fixedBreaker{tripped: true},
	})
	status, _, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusOK {
		t.Fatalf("status=%d — loop check must be skipped without session id", status)
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "completed" {
		t.Fatalf("usage=%q", ev.Status)
	}
}

func TestGuardLoopDetected(t *testing.T) {
	hits := &hitCountingUpstream{
		next: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Fatal("upstream must not be called when loop is already tripped")
		}),
	}
	h := newHarness(t, harnessOpts{
		guardEnabled: true,
		mgmtEnabled:  true,
		budgetLimit:  10_000_000,
		breaker:      fixedBreaker{tripped: true},
		upstream:     hits,
	})
	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(map[string]string{
		"X-TokenGuard-Session-ID": "sess-loop-1",
	}))
	if status != http.StatusConflict || data["code"] != "loop_detected" {
		t.Fatalf("status=%d data=%v", status, data)
	}
	if hits.Hits() != 0 {
		t.Fatalf("upstream hits = %d", hits.Hits())
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "blocked_loop" {
		t.Fatalf("usage=%q", ev.Status)
	}
}

func TestGuardLoopCheckUnavailable(t *testing.T) {
	h := newHarness(t, harnessOpts{
		guardEnabled: true,
		mgmtEnabled:  true,
		budgetLimit:  10_000_000,
		breaker:      fixedBreaker{err: fmt.Errorf("redis down")},
	})
	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(map[string]string{
		"X-TokenGuard-Session-ID": "sess-1",
	}))
	if status != http.StatusServiceUnavailable || data["code"] != "loop_check_unavailable" {
		t.Fatalf("status=%d data=%v", status, data)
	}
}

func TestGuardBudgetStoreUnavailable(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, budgetLimit: 10_000_000})
	h.store.failLookup = true
	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusServiceUnavailable || data["code"] != "budget_unavailable" {
		t.Fatalf("status=%d data=%v", status, data)
	}
}

func TestGuardProviderErrorNoCharge(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream boom"}`))
	})
	h := newHarness(t, harnessOpts{
		guardEnabled: true,
		mgmtEnabled:  true,
		budgetLimit:  10_000_000,
		upstream:     upstream,
	})
	status, _, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusInternalServerError {
		t.Fatalf("status=%d", status)
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "provider_error" {
		t.Fatalf("usage=%q", ev.Status)
	}
	if ev.ActualCostMicroUSD != 0 {
		t.Fatalf("actual cost = %d, want 0", ev.ActualCostMicroUSD)
	}
}

func TestGuardAltAPIKeyHeader(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, budgetLimit: 10_000_000})
	status, _, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), map[string]string{
		"Content-Type":          "application/json",
		"Authorization":         "Bearer sk-test",
		"X-TokenGuard-Key":      h.apiKey,
		"X-TokenGuard-Provider": "openai",
	})
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "completed" {
		t.Fatalf("usage=%q", ev.Status)
	}
}
