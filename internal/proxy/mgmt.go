package proxy

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
)

type provisionRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type provisionResponse struct {
	UserID      string         `json:"user_id"`
	APIKey      string         `json:"api_key"`
	APIKeyID    string         `json:"api_key_id"`
	Integration map[string]any `json:"integration"`
}

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

	userID, err := h.budgetStore.CreateUser(r.Context(), req.Email, req.Name)
	if err != nil {
		writeManagementJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user: " + err.Error()})
		return
	}

	keyID, plaintext, err := h.budgetStore.CreateAPIKey(r.Context(), userID, "default")
	if err != nil {
		writeManagementJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create API key: " + err.Error()})
		return
	}

	writeManagementJSON(w, http.StatusCreated, provisionResponse{
		UserID:   userID,
		APIKey:   plaintext,
		APIKeyID: keyID,
		Integration: map[string]any{
			"docs_url":      "/docs",
			"dashboard_url": "/dashboard",
			"discovery_url": "/v1/tokenguard.json",
			"next_steps": []string{
				"Set your SDK baseURL to this host + /v1",
				"Send X-TokenGuard-API-Key with the api_key from this response",
				"Keep sending your real provider API key as usual",
				"Add X-TokenGuard-Session-ID for agent runs",
			},
			"required_headers": []string{
				"X-TokenGuard-API-Key",
				"Authorization or x-api-key (provider)",
			},
		},
	})
}

func (h *Handler) authorizedManagementRequest(r *http.Request) bool {
	secret := strings.TrimSpace(r.Header.Get("X-TokenGuard-Admin-Secret"))
	if h == nil || h.adminSecret == "" || len(secret) != len(h.adminSecret) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(secret), []byte(h.adminSecret)) == 1
}
