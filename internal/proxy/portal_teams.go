package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"tokenguard/internal/billing"
)

func (h *Handler) HandlePortalCreateTeam(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Name      string   `json:"name"`
		BudgetUSD *float64 `json:"budget_usd"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	limit := h.portalDefaultBudgetMicroUSD
	if req.BudgetUSD != nil {
		if *req.BudgetUSD < 0 {
			writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "budget_usd must be >= 0"})
			return
		}
		limit = usdToMicro(*req.BudgetUSD)
	}
	team, err := h.accountStore.CreateTeam(r.Context(), userID, name, limit)
	if err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writePortalJSON(w, http.StatusCreated, team)
}

func (h *Handler) HandlePortalListTeams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	teams, err := h.accountStore.ListTeamsForUser(r.Context(), userID)
	if err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list teams"})
		return
	}
	if teams == nil {
		teams = []billing.Team{}
	}
	writePortalJSON(w, http.StatusOK, map[string]any{"teams": teams})
}

func (h *Handler) HandlePortalUpdateTeamBudget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	var req struct {
		TeamID    string   `json:"team_id"`
		BudgetUSD *float64 `json:"budget_usd"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || strings.TrimSpace(req.TeamID) == "" || req.BudgetUSD == nil {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "team_id and budget_usd are required"})
		return
	}
	if *req.BudgetUSD < 0 {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "budget_usd must be >= 0"})
		return
	}
	team, err := h.accountStore.UpdateTeamBudget(r.Context(), userID, req.TeamID, usdToMicro(*req.BudgetUSD))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, billing.ErrNotTeamOwner) || errors.Is(err, billing.ErrTeamNotFound) {
			status = http.StatusForbidden
		}
		writePortalJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writePortalJSON(w, http.StatusOK, team)
}

func (h *Handler) HandlePortalAddTeamMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	var req struct {
		TeamID string   `json:"team_id"`
		Email  string   `json:"email"`
		CapUSD *float64 `json:"cap_usd"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.TeamID) == "" || strings.TrimSpace(req.Email) == "" || req.CapUSD == nil {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "team_id, email, and cap_usd are required"})
		return
	}
	if *req.CapUSD < 0 {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "cap_usd must be >= 0"})
		return
	}
	member, err := h.accountStore.AddTeamMemberByEmail(r.Context(), userID, req.TeamID, req.Email, usdToMicro(*req.CapUSD))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, billing.ErrNotTeamOwner) {
			status = http.StatusForbidden
		}
		writePortalJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writePortalJSON(w, http.StatusCreated, member)
}

func (h *Handler) HandlePortalUpdateMemberCap(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	var req struct {
		TeamID string   `json:"team_id"`
		UserID string   `json:"user_id"`
		CapUSD *float64 `json:"cap_usd"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil ||
		strings.TrimSpace(req.TeamID) == "" || strings.TrimSpace(req.UserID) == "" || req.CapUSD == nil {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "team_id, user_id, and cap_usd are required"})
		return
	}
	member, err := h.accountStore.UpdateTeamMemberCap(r.Context(), userID, req.TeamID, req.UserID, usdToMicro(*req.CapUSD))
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, billing.ErrNotTeamOwner) {
			status = http.StatusForbidden
		}
		writePortalJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writePortalJSON(w, http.StatusOK, member)
}

func (h *Handler) HandlePortalRemoveTeamMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	var req struct {
		TeamID string `json:"team_id"`
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil ||
		strings.TrimSpace(req.TeamID) == "" || strings.TrimSpace(req.UserID) == "" {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "team_id and user_id are required"})
		return
	}
	if err := h.accountStore.RemoveTeamMember(r.Context(), userID, req.TeamID, req.UserID); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, billing.ErrNotTeamOwner) {
			status = http.StatusForbidden
		}
		writePortalJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writePortalJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandlePortalListTeamMembers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	teamID := strings.TrimSpace(r.URL.Query().Get("team_id"))
	if teamID == "" {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "team_id query param is required"})
		return
	}
	members, err := h.accountStore.ListTeamMembers(r.Context(), userID, teamID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, billing.ErrTeamNotFound) {
			status = http.StatusForbidden
		}
		writePortalJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writePortalJSON(w, http.StatusOK, map[string]any{"members": members})
}
