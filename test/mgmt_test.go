package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMgmtUnauthorized(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})

	status, data, _ := h.doJSON(http.MethodGet, "/mgmt/users", nil, map[string]string{
		"Content-Type": "application/json",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 data=%v", status, data)
	}

	status, data, _ = h.doJSON(http.MethodGet, "/mgmt/users", nil, map[string]string{
		"Content-Type":              "application/json",
		"X-TokenGuard-Admin-Secret": "wrong-secret-that-is-long",
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("wrong secret status = %d data=%v", status, data)
	}
}

func TestMgmtOptionsCORS(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})
	status, _, hdr := h.do(http.MethodOptions, "/mgmt/users", nil, nil)
	if status != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d, want 204", status)
	}
	if hdr.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("CORS origin = %q", hdr.Get("Access-Control-Allow-Origin"))
	}

	status, _, hdr = h.do(http.MethodGet, "/mgmt/users", nil, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if hdr.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("CORS origin = %q", hdr.Get("Access-Control-Allow-Origin"))
	}
}

func TestMgmtProvisionAndBudget(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})

	status, data, _ := h.doJSON(http.MethodPost, "/mgmt/provision", loadFixture(t, "mgmt.provision.json"), h.adminHeaders())
	if status != http.StatusCreated {
		t.Fatalf("provision status = %d data=%v", status, data)
	}
	apiKey, _ := data["api_key"].(string)
	userID, _ := data["user_id"].(string)
	if !strings.HasPrefix(apiKey, "tg_") {
		t.Fatalf("api_key = %q", apiKey)
	}
	if lim, ok := data["limit_microusd"].(float64); !ok || lim != 1_000_000 {
		t.Fatalf("limit_microusd = %#v", data["limit_microusd"])
	}
	if _, ok := data["integration"]; !ok {
		t.Fatal("missing integration hints")
	}

	budgetBody := map[string]any{
		"user_id":     userID,
		"budget_usd":  5,
		"reset_spent": false,
	}
	status, data, _ = h.doJSON(http.MethodPatch, "/mgmt/budget", budgetBody, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("budget status = %d data=%v", status, data)
	}
	if data["budget_usd"] != float64(5) {
		t.Fatalf("budget_usd = %v", data["budget_usd"])
	}

	status, data, _ = h.doJSON(http.MethodGet, "/mgmt/users", nil, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("users status = %d", status)
	}
	users, ok := data["users"].([]any)
	if !ok || len(users) == 0 {
		t.Fatalf("users = %#v", data["users"])
	}

	status, data, _ = h.doJSON(http.MethodPost, "/mgmt/provision", map[string]any{"name": "x"}, h.adminHeaders())
	if status != http.StatusBadRequest {
		t.Fatalf("missing email status = %d data=%v", status, data)
	}

	status, _, _ = h.do(http.MethodGet, "/mgmt/provision", nil, h.adminHeaders())
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("GET provision status = %d, want 405", status)
	}
}

func TestMgmtBudgetResetSpent(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, budgetLimit: 100})
	h.store.mu.Lock()
	b := h.store.budgets[h.userID]
	b.SpentMicroUSD = 50
	h.store.budgets[h.userID] = b
	h.store.mu.Unlock()

	status, data, _ := h.doJSON(http.MethodPost, "/mgmt/budget", map[string]any{
		"user_id":     h.userID,
		"budget_usd":  2,
		"reset_spent": true,
	}, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("status = %d data=%v", status, data)
	}
	user, _ := data["user"].(map[string]any)
	if user["spent_microusd"] != float64(0) {
		t.Fatalf("spent not reset: %#v", user)
	}
}

func TestMgmtUsageAfterChat(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, budgetLimit: 10_000_000})
	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", loadFixture(t, "chat.request.json"), h.proxyHeaders(nil))
	if status != http.StatusOK {
		t.Fatalf("chat status = %d data=%v", status, data)
	}
	ev := h.waitUsage(2 * time.Second)
	if ev.Status != "completed" {
		t.Fatalf("usage status = %q", ev.Status)
	}

	status, data, _ = h.doJSON(http.MethodGet, "/mgmt/usage?limit=10", nil, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("usage list status = %d data=%v", status, data)
	}
	events, ok := data["events"].([]any)
	if !ok || len(events) == 0 {
		t.Fatalf("events = %#v", data["events"])
	}
}
