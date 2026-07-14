package proxy

import (
	"net/http"
	"strconv"
)

func (h *Handler) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodGet {
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

	users, err := h.budgetStore.ListUsers(r.Context())
	if err != nil {
		writeManagementJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list users: " + err.Error()})
		return
	}

	writeManagementJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (h *Handler) HandleListUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodGet {
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

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	events, err := h.budgetStore.ListRecentUsage(r.Context(), limit)
	if err != nil {
		writeManagementJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list usage: " + err.Error()})
		return
	}

	writeManagementJSON(w, http.StatusOK, map[string]any{"events": events})
}
