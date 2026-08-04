package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net/http"
	"strings"

	"tokenguard/internal/billing"
)

const sessionCookieName = "tokenguard_session"

type portalMeResponse struct {
	User        billing.AccountView `json:"user"`
	Integration map[string]any      `json:"integration"`
	Limits      map[string]any      `json:"limits"`
}

func (h *Handler) portalEnabled() bool {
	return h != nil && h.portalEnabledFlag && h.accountStore != nil
}

func (h *Handler) clerkConfigured() bool {
	return h != nil && strings.TrimSpace(h.clerkSecretKey) != ""
}

func (h *Handler) HandlePortalPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.portalEnabled() {
		http.NotFound(w, r)
		return
	}
	app := strings.TrimSpace(h.portalAppURL)
	if app == "" {
		writePortalJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Portal UI is the Next.js app. Set TOKENGUARD_PORTAL_APP_URL (e.g. http://localhost:3000/portal).",
			"code":  "portal_ui_not_configured",
		})
		return
	}
	http.Redirect(w, r, app, http.StatusFound)
}

func (h *Handler) HandlePortalDevLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.portalEnabled() || !h.portalDevLogin {
		http.NotFound(w, r)
		return
	}

	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	name := strings.TrimSpace(req.Name)
	if email == "" {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "Email is required"})
		return
	}
	if name == "" {
		name = strings.Split(email, "@")[0]
	}

	userID, _, err := h.accountStore.EnsureOAuthUser(
		r.Context(), "dev", email, email, name, h.portalDefaultBudgetMicroUSD,
	)
	if err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create account"})
		return
	}
	if err := h.issueSession(w, userID); err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create session"})
		return
	}
	writePortalJSON(w, http.StatusOK, map[string]string{"status": "ok", "user_id": userID})
}

func (h *Handler) HandlePortalLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.portalEnabled() {
		http.NotFound(w, r)
		return
	}
	if token := h.sessionToken(r); token != "" {
		_ = h.accountStore.RevokeAuthSession(r.Context(), token)
	}
	h.clearSessionCookie(w)
	if r.Method == http.MethodGet {
		http.Redirect(w, r, "/portal", http.StatusFound)
		return
	}
	writePortalJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) HandlePortalMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	view, err := h.accountStore.GetAccountView(r.Context(), userID)
	if err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load account"})
		return
	}
	writePortalJSON(w, http.StatusOK, portalMeResponse{
		User: view,
		Integration: map[string]any{
			"proxy_base_url": h.publicBaseURL(r) + "/v1",
			"proxy_url":      h.publicBaseURL(r) + "/v1/chat/completions",
			"docs_url":       "/docs",
			"discovery_url":  "/v1/tokenguard.json",
			"api_key_header": "X-TokenGuard-API-Key",
		},
		Limits: map[string]any{
			"max_keys":           h.portalMaxKeys,
			"default_budget_usd": float64(h.portalDefaultBudgetMicroUSD) / 1_000_000,
			"can_create_key":     view.ActiveKeyCount < h.portalMaxKeys,
		},
	})
}

func (h *Handler) HandlePortalCreateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}

	count, err := h.accountStore.CountActiveAPIKeys(r.Context(), userID)
	if err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to check keys"})
		return
	}
	if count >= h.portalMaxKeys {
		writePortalJSON(w, http.StatusConflict, map[string]string{
			"error": "Maximum number of API keys reached",
			"code":  "max_keys_reached",
		})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "default"
	}

	keyID, plaintext, err := h.accountStore.CreateAPIKey(r.Context(), userID, name)
	if err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create API key"})
		return
	}
	writePortalJSON(w, http.StatusCreated, map[string]any{
		"api_key_id": keyID,
		"api_key":    plaintext,
		"name":       name,
		"note":       "Copy this key now. TokenGuard stores only a hash and will not show the full key again.",
	})
}

func (h *Handler) HandlePortalRevokeKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := h.requirePortalUser(w, r)
	if !ok {
		return
	}
	var req struct {
		KeyID string `json:"key_id"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || strings.TrimSpace(req.KeyID) == "" {
		writePortalJSON(w, http.StatusBadRequest, map[string]string{"error": "key_id is required"})
		return
	}
	if err := h.accountStore.RevokeAPIKey(r.Context(), userID, req.KeyID); err != nil {
		writePortalJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writePortalJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) requirePortalUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	if !h.portalEnabled() {
		http.NotFound(w, r)
		return "", false
	}

	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth != "" {
		if !h.clerkConfigured() {
			writePortalJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Clerk is not configured on the API (TOKENGUARD_CLERK_SECRET_KEY)",
				"code":  "clerk_not_configured",
			})
			return "", false
		}
		identity, err := h.verifyClerkBearer(r.Context(), auth)
		if err != nil {
			writePortalJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Invalid Clerk session",
				"code":  "unauthorized",
			})
			return "", false
		}
		userID, _, err := h.accountStore.EnsureOAuthUser(
			r.Context(),
			"clerk",
			identity.Subject,
			identity.Email,
			identity.Name,
			h.portalDefaultBudgetMicroUSD,
		)
		if err != nil {
			log.Printf("portal clerk ensure user: %v", err)
			writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to resolve account"})
			return "", false
		}
		return userID, true
	}

	// Cookie sessions: harness / TOKENGUARD_PORTAL_DEV_LOGIN only.
	token := h.sessionToken(r)
	if token == "" {
		writePortalJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Not signed in",
			"code":  "unauthorized",
		})
		return "", false
	}
	sess, err := h.accountStore.LookupAuthSession(r.Context(), token)
	if err != nil {
		if errors.Is(err, billing.ErrSessionNotFound) || errors.Is(err, billing.ErrSessionExpired) {
			h.clearSessionCookie(w)
			writePortalJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Session expired or invalid",
				"code":  "unauthorized",
			})
			return "", false
		}
		writePortalJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Session store unavailable"})
		return "", false
	}
	return sess.UserID, true
}

func (h *Handler) issueSession(w http.ResponseWriter, userID string) error {
	token, err := h.accountStore.CreateAuthSession(context.Background(), userID, h.portalSessionTTL)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(h.portalSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.portalSecureCookies,
	})
	return nil
}

func writePortalJSON(w http.ResponseWriter, status int, payload any) {
	setPortalCORSHeaders(w, "")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (h *Handler) setPortalCORS(w http.ResponseWriter, r *http.Request) {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	setPortalCORSHeaders(w, origin)
	if origin == "" {
		return
	}
	if h.portalOriginAllowed(origin) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		return
	}
	// Harness / local cookie tests may omit CORS allowlist.
	if h.portalDevLogin && len(h.portalCORSOrigins) == 0 {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
}

func (h *Handler) portalOriginAllowed(origin string) bool {
	want := strings.TrimRight(strings.TrimSpace(origin), "/")
	for _, allowed := range h.portalCORSOrigins {
		if strings.TrimRight(strings.TrimSpace(allowed), "/") == want {
			return true
		}
	}
	return false
}

func setPortalCORSHeaders(w http.ResponseWriter, _ string) {
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Max-Age", "86400")
}

func (h *Handler) HandlePortalOptions(w http.ResponseWriter, r *http.Request) {
	// Always attach CORS on preflight so browsers surface API errors instead of a opaque CORS failure.
	h.setPortalCORS(w, r)
	if !h.portalEnabled() {
		writePortalJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Portal is disabled on this API host",
			"code":  "portal_disabled",
		})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) WithPortalCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			h.HandlePortalOptions(w, r)
			return
		}
		h.setPortalCORS(w, r)
		next(w, r)
	}
}

func (h *Handler) sessionToken(r *http.Request) string {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(c.Value)
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.portalSecureCookies,
	})
}

func (h *Handler) publicBaseURL(r *http.Request) string {
	if base := strings.TrimRight(strings.TrimSpace(h.portalBaseURL), "/"); base != "" {
		return base
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func usdToMicro(usd float64) int64 {
	return int64(math.Round(usd * 1_000_000))
}
