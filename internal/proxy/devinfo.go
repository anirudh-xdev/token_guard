package proxy

import (
	"net/http"
	"sort"
	"strings"
)

// HandleDevInfo exposes a public, non-secret discovery document so developers
// can learn how to integrate without reading the repo.
func (h *Handler) HandleDevInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	providers := make([]string, 0, len(h.providerRoutes))
	providerBases := make(map[string]string, len(h.providerRoutes))
	for name, route := range h.providerRoutes {
		providers = append(providers, name)
		if route.Upstream != nil {
			providerBases[name] = strings.TrimRight(route.Upstream.String(), "/")
		}
	}
	sort.Strings(providers)

	var models []string
	if h.pricing != nil {
		models = h.pricing.ModelNames()
	}

	payload := map[string]any{
		"service":             "tokenguard",
		"description":         "Financial firewall proxy for LLM APIs. Point your SDK base URL here.",
		"guard_enabled":       h.budgetStore != nil && h.pricing != nil && h.circuitBreaker != nil,
		"management_enabled":  h.managementEnabled,
		"default_provider":    h.defaultProvider,
		"providers":           providers,
		"provider_bases":      providerBases,
		"models_priced":       models,
		"docs_url":            "/docs",
		"dashboard_url":       "/dashboard",
		"health_url":          "/healthz",
		"proxy_base_url_hint": "Use this host as your OpenAI-compatible base URL (usually .../v1).",
		"openrouter_note":     "OpenRouter upstream base should be https://openrouter.ai/api (not .../api/v1).",
		"required_headers": map[string]string{
			"X-TokenGuard-API-Key":      "User key from /dashboard or POST /mgmt/provision (tg_...)",
			"Authorization / x-api-key": "Your real provider API key (passed through)",
			"X-TokenGuard-Provider":     "Optional. One of the providers list (e.g. openai, openrouter)",
			"X-TokenGuard-Session-ID":   "Recommended for agents — enables loop detection",
		},
		"admin_header": "X-TokenGuard-Admin-Secret",
		"mgmt_routes": []string{
			"POST /mgmt/provision",
			"PATCH /mgmt/budget",
			"GET /mgmt/users",
			"GET /mgmt/usage",
			"GET /mgmt/pricing",
			"POST /mgmt/pricing/upsert",
			"POST /mgmt/pricing/delete",
			"POST /mgmt/pricing/sync/openrouter",
		},
		"status_codes": map[string]string{
			"401": "Missing or invalid TokenGuard API key",
			"400": "Bad request or model missing from pricing",
			"402": "Budget exceeded — operator can PATCH /mgmt/budget to extend",
			"409": "Agent loop detected",
			"413": "Request body too large",
			"503": "Billing or loop service unavailable",
		},
		"quickstart": []string{
			"1. Open /docs for the human guide",
			"2. Unlock /dashboard with your admin secret",
			"3. Provision a user (set budget_usd) and copy the tg_ API key",
			"4. Set your SDK baseURL to this host + /v1",
			"5. Send provider auth + X-TokenGuard-API-Key on every request",
		},
	}

	writeJSON(w, http.StatusOK, payload)
}
