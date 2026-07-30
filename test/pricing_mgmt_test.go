package e2e

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPricingListUpsertDelete(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true, budgetLimit: 10_000_000})

	status, data, _ := h.doJSON(http.MethodGet, "/mgmt/pricing", nil, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("list status = %d data=%v", status, data)
	}
	count, _ := data["count"].(float64)
	if count < 1 {
		t.Fatalf("expected seeded prices, count=%v", data["count"])
	}

	status, data, _ = h.doJSON(http.MethodPost, "/mgmt/pricing/upsert", loadFixture(t, "mgmt.upsert.json"), h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("upsert status = %d data=%v", status, data)
	}
	price, _ := data["price"].(map[string]any)
	if price["model_key"] != "gpt-e2e-custom" {
		t.Fatalf("price = %#v", price)
	}
	if price["input_usd_per_million"] != 0.15 {
		t.Fatalf("input_usd_per_million = %v", price["input_usd_per_million"])
	}

	// Proxy should accept the new model once in-memory engine is updated by upsert handler.
	req := []byte(`{"model":"gpt-e2e-custom","messages":[{"role":"user","content":"hi"}],"max_tokens":8}`)
	status, _, _ = h.doJSON(http.MethodPost, "/v1/chat/completions", req, h.proxyHeaders(nil))
	if status != http.StatusOK {
		t.Fatalf("chat with custom model status = %d", status)
	}
	_ = h.waitUsage(2 * time.Second)

	status, data, _ = h.doJSON(http.MethodPost, "/mgmt/pricing/delete", map[string]any{"model_key": "gpt-e2e-custom"}, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("delete status = %d data=%v", status, data)
	}

	status, data, _ = h.doJSON(http.MethodPost, "/v1/chat/completions", req, h.proxyHeaders(nil))
	if status != http.StatusBadRequest {
		t.Fatalf("after delete status = %d, want 400 data=%v", status, data)
	}
	if data["code"] != "pricing_not_configured" {
		t.Fatalf("code = %v", data["code"])
	}

	status, data, _ = h.doJSON(http.MethodPost, "/mgmt/pricing/delete", map[string]any{"model_key": "no-such-model"}, h.adminHeaders())
	if status != http.StatusNotFound {
		t.Fatalf("delete missing status = %d data=%v", status, data)
	}
}

func TestPricingSyncOpenRouterMock(t *testing.T) {
	modelsBody := loadFixture(t, "openrouter.models.json")
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(modelsBody)
	}))
	t.Cleanup(or.Close)
	t.Setenv("TOKENGUARD_OPENROUTER_MODELS_URL", or.URL)

	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})

	status, data, _ := h.doJSON(http.MethodPost, "/mgmt/pricing/sync/openrouter", nil, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("sync status = %d data=%v", status, data)
	}
	imported, _ := data["imported"].(float64)
	if imported < 1 {
		t.Fatalf("imported = %v", data["imported"])
	}

	// Synced leaf + openrouter/id should be in catalog
	status, data, _ = h.doJSON(http.MethodGet, "/mgmt/pricing", nil, h.adminHeaders())
	if status != http.StatusOK {
		t.Fatalf("list status = %d", status)
	}
	prices, _ := data["prices"].([]any)
	found := false
	for _, p := range prices {
		m, _ := p.(map[string]any)
		if m["model_key"] == "gpt-e2e-sync" || m["model_key"] == "openai/gpt-e2e-sync" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("synced model not in catalog: %#v", data["prices"])
	}

	// Unknown model still fail-closed after successful sync of other models.
	req := []byte(`{"model":"totally-unknown-model-xyz","messages":[{"role":"user","content":"hi"}],"max_tokens":8}`)
	status, data, _ = h.doJSON(http.MethodPost, "/v1/chat/completions", req, h.proxyHeaders(nil))
	if status != http.StatusBadRequest {
		t.Fatalf("unknown model status = %d data=%v", status, data)
	}
	if data["code"] != "pricing_not_configured" {
		t.Fatalf("code = %v", data["code"])
	}
}

func TestPricingSyncOpenRouterFailure(t *testing.T) {
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"nope"}`))
	}))
	t.Cleanup(or.Close)
	t.Setenv("TOKENGUARD_OPENROUTER_MODELS_URL", or.URL)

	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})
	status, data, _ := h.doJSON(http.MethodPost, "/mgmt/pricing/sync/openrouter", nil, h.adminHeaders())
	if status != http.StatusBadGateway {
		t.Fatalf("status = %d data=%v", status, data)
	}
}
