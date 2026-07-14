package proxy

import (
	"crypto/subtle"
	"encoding/json"
	"math"
	"net/http"
	"strings"
)

type provisionRequest struct {
	Email         string   `json:"email"`
	Name          string   `json:"name"`
	BudgetUSD     *float64 `json:"budget_usd"`
	LimitMicroUSD *int64   `json:"limit_microusd"`
}

type provisionResponse struct {
	UserID          string         `json:"user_id"`
	APIKey          string         `json:"api_key"`
	APIKeyID        string         `json:"api_key_id"`
	LimitMicroUSD   int64          `json:"limit_microusd"`
	BudgetUSD       float64        `json:"budget_usd"`
	Integration     map[string]any `json:"integration"`
}

type updateBudgetRequest struct {
	UserID        string   `json:"user_id"`
	BudgetUSD     *float64 `json:"budget_usd"`
	LimitMicroUSD *int64   `json:"limit_microusd"`
	ResetSpent    bool     `json:"reset_spent"`
}

func resolveLimitMicroUSD(budgetUSD *float64, limitMicroUSD *int64) (int64, error) {
	if limitMicroUSD != nil {
		if *limitMicroUSD < 0 {
			return 0, errInvalidBudget
		}
		return *limitMicroUSD, nil
	}
	if budgetUSD != nil {
		if *budgetUSD < 0 {
			return 0, errInvalidBudget
		}
		return int64(math.Round(*budgetUSD * 1_000_000)), nil
	}
	return 0, nil // caller uses default
}

var errInvalidBudget = jsonError("budget must be >= 0")

type jsonError string

func (e jsonError) Error() string { return string(e) }

func (h *Handler) HandleProvision(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.authorizedManagementRequest(r) {
		writeManagementJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized management access"})
		return
	}
	if h.budgetStore == nil {
		writeManagementJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Management store unavailable"})
		return
	}

	var req provisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Email == "" {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": "Email is required"})
		return
	}

	limit, err := resolveLimitMicroUSD(req.BudgetUSD, req.LimitMicroUSD)
	if err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	userID, err := h.budgetStore.CreateUser(r.Context(), req.Email, req.Name, limit)
	if err != nil {
		writeManagementJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user: " + err.Error()})
		return
	}

	keyID, plaintext, err := h.budgetStore.CreateAPIKey(r.Context(), userID, "default")
	if err != nil {
		writeManagementJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create API key: " + err.Error()})
		return
	}

	if limit <= 0 {
		limit = 1_000_000
	}

	writeManagementJSON(w, http.StatusCreated, provisionResponse{
		UserID:        userID,
		APIKey:        plaintext,
		APIKeyID:      keyID,
		LimitMicroUSD: limit,
		BudgetUSD:     float64(limit) / 1_000_000,
		Integration: map[string]any{
			"docs_url":      "/docs",
			"dashboard_url": "/dashboard",
			"discovery_url": "/v1/tokenguard.json",
			"proxy_url":     "/v1/chat/completions",
			"next_steps": []string{
				"Set your SDK baseURL to this host + /v1",
				"Send X-TokenGuard-API-Key with the api_key from this response",
				"Keep sending your real provider API key as usual",
				"Add X-TokenGuard-Session-ID for agent runs",
				"When the user hits budget, PATCH /mgmt/budget to extend the limit",
			},
			"required_headers": []string{
				"X-TokenGuard-API-Key",
				"Authorization or x-api-key (provider)",
			},
		},
	})
}

// HandleUpdateBudget extends or changes a user's spend limit (operator-only).
// POST/PATCH /mgmt/budget  body: {user_id, budget_usd|limit_microusd, reset_spent?}
func (h *Handler) HandleUpdateBudget(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodPatch && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.authorizedManagementRequest(r) {
		writeManagementJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized management access"})
		return
	}
	if h.budgetStore == nil {
		writeManagementJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Management store unavailable"})
		return
	}

	var req updateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.UserID) == "" {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}
	if req.BudgetUSD == nil && req.LimitMicroUSD == nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": "budget_usd or limit_microusd is required"})
		return
	}

	limit, err := resolveLimitMicroUSD(req.BudgetUSD, req.LimitMicroUSD)
	if err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if limit <= 0 {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": "budget must be greater than 0"})
		return
	}

	view, err := h.budgetStore.UpdateUserBudget(r.Context(), req.UserID, limit, req.ResetSpent)
	if err != nil {
		if strings.Contains(err.Error(), "user not found") {
			writeManagementJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeManagementJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update budget: " + err.Error()})
		return
	}

	writeManagementJSON(w, http.StatusOK, map[string]any{
		"user":             view,
		"budget_usd":       float64(view.LimitMicroUSD) / 1_000_000,
		"available_usd":    float64(view.LimitMicroUSD-view.SpentMicroUSD) / 1_000_000,
		"reset_spent":      req.ResetSpent,
	})
}

func (h *Handler) authorizedManagementRequest(r *http.Request) bool {
	secret := strings.TrimSpace(r.Header.Get("X-TokenGuard-Admin-Secret"))
	if h == nil || h.adminSecret == "" || len(secret) != len(h.adminSecret) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(secret), []byte(h.adminSecret)) == 1
}
