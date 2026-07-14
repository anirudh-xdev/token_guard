package proxy

import (
	"net/http"
	"sort"
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
	for name := range h.providerRoutes {
		providers = append(providers, name)
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
		"models_priced":       models,
		"docs_url":            "/docs",
		"dashboard_url":       "/dashboard",
		"health_url":          "/healthz",
		"proxy_base_url_hint": "Use this host as your OpenAI-compatible base URL (usually .../v1).",
		"required_headers": map[string]string{
			"X-TokenGuard-API-Key":    "User key from /dashboard or POST /mgmt/provision (tg_...)",
			"Authorization / x-api-key": "Your real provider API key (passed through)",
			"X-TokenGuard-Provider":   "Optional. One of the providers list (e.g. openai, openrouter)",
			"X-TokenGuard-Session-ID": "Recommended for agents — enables loop detection",
		},
		"admin_header": "X-TokenGuard-Admin-Secret",
		"status_codes": map[string]string{
			"401": "Missing or invalid TokenGuard API key",
			"400": "Bad request or model missing from pricing",
			"402": "Budget exceeded",
			"409": "Agent loop detected",
			"413": "Request body too large",
			"503": "Billing or loop service unavailable",
		},
		"quickstart": []string{
			"1. Open /docs for the human guide",
			"2. Unlock /dashboard with your admin secret",
			"3. Provision a user and copy the tg_ API key",
			"4. Set your SDK baseURL to this host + /v1",
			"5. Send provider auth + X-TokenGuard-API-Key on every request",
		},
	}

	writeJSON(w, http.StatusOK, payload)
}
