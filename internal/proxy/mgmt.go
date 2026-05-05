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
	UserID   string `json:"user_id"`
	APIKey   string `json:"api_key"`
	APIKeyID string `json:"api_key_id"`
}

func (h *Handler) HandleProvision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !h.authorizedManagementRequest(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized management access"})
		return
	}
	if h.budgetStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Management store unavailable"})
		return
	}

	var req provisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Email is required"})
		return
	}

	userID, err := h.budgetStore.CreateUser(r.Context(), req.Email, req.Name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user: " + err.Error()})
		return
	}

	keyID, plaintext, err := h.budgetStore.CreateAPIKey(r.Context(), userID, "default")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create API key: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, provisionResponse{
		UserID:   userID,
		APIKey:   plaintext,
		APIKeyID: keyID,
	})
}

func (h *Handler) authorizedManagementRequest(r *http.Request) bool {
	secret := strings.TrimSpace(r.Header.Get("X-TokenGuard-Admin-Secret"))
	if h == nil || h.adminSecret == "" || len(secret) != len(h.adminSecret) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(secret), []byte(h.adminSecret)) == 1
}
