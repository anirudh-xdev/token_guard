package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"tokenguard/internal/billing"
)

const (
	sessionCookieName = "tokenguard_session"
	oauthStateCookie  = "tokenguard_oauth_state"
)

type portalMeResponse struct {
	User        billing.AccountView `json:"user"`
	Integration map[string]any      `json:"integration"`
	Limits      map[string]any      `json:"limits"`
}

func (h *Handler) portalEnabled() bool {
	return h != nil && h.portalEnabledFlag && h.accountStore != nil
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
	if len(h.portalHTML) == 0 {
		http.Error(w, "Portal UI unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(h.portalHTML)
}

func (h *Handler) HandlePortalGitHubLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.portalEnabled() {
		http.NotFound(w, r)
		return
	}
	if strings.TrimSpace(h.githubClientID) == "" || strings.TrimSpace(h.githubClientSecret) == "" {
		writePortalJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "GitHub OAuth is not configured",
			"code":  "oauth_not_configured",
		})
		return
	}

	state, err := randomToken(16)
	if err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start login"})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		MaxAge:   600,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.portalSecureCookies,
	})

	q := url.Values{}
	q.Set("client_id", h.githubClientID)
	q.Set("redirect_uri", h.githubCallbackURL())
	q.Set("scope", "read:user user:email")
	q.Set("state", state)
	http.Redirect(w, r, "https://github.com/login/oauth/authorize?"+q.Encode(), http.StatusFound)
}

func (h *Handler) HandlePortalGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.portalEnabled() {
		http.NotFound(w, r)
		return
	}

	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if state == "" || code == "" {
		http.Redirect(w, r, "/portal?error=oauth_denied", http.StatusFound)
		return
	}
	cookie, err := r.Cookie(oauthStateCookie)
	if err != nil || cookie.Value == "" || cookie.Value != state {
		http.Redirect(w, r, "/portal?error=oauth_state", http.StatusFound)
		return
	}
	// Clear state cookie
	http.SetCookie(w, &http.Cookie{Name: oauthStateCookie, Value: "", Path: "/", MaxAge: -1})

	ghUser, err := exchangeGitHubCode(r.Context(), h.githubClientID, h.githubClientSecret, code, h.githubCallbackURL())
	if err != nil {
		log.Printf("portal github oauth: %v", err)
		http.Redirect(w, r, "/portal?error=oauth_failed", http.StatusFound)
		return
	}

	userID, _, err := h.accountStore.EnsureOAuthUser(
		r.Context(),
		"github",
		ghUser.Subject,
		ghUser.Email,
		ghUser.Name,
		h.portalDefaultBudgetMicroUSD,
	)
	if err != nil {
		log.Printf("portal ensure user: %v", err)
		http.Redirect(w, r, "/portal?error=account_failed", http.StatusFound)
		return
	}

	if err := h.issueSession(w, r, userID); err != nil {
		log.Printf("portal session: %v", err)
		http.Redirect(w, r, "/portal?error=session_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/portal", http.StatusFound)
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
		r.Context(),
		"dev",
		email,
		email,
		name,
		h.portalDefaultBudgetMicroUSD,
	)
	if err != nil {
		writePortalJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create account"})
		return
	}
	if err := h.issueSession(w, r, userID); err != nil {
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
	sess, ok := h.requirePortalSession(w, r)
	if !ok {
		return
	}
	view, err := h.accountStore.GetAccountView(r.Context(), sess.UserID)
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
			"max_keys":              h.portalMaxKeys,
			"default_budget_usd":    float64(h.portalDefaultBudgetMicroUSD) / 1_000_000,
			"can_create_key":        view.ActiveKeyCount < h.portalMaxKeys,
		},
	})
}

func (h *Handler) HandlePortalCreateKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sess, ok := h.requirePortalSession(w, r)
	if !ok {
		return
	}

	count, err := h.accountStore.CountActiveAPIKeys(r.Context(), sess.UserID)
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

	keyID, plaintext, err := h.accountStore.CreateAPIKey(r.Context(), sess.UserID, name)
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
	sess, ok := h.requirePortalSession(w, r)
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
	if err := h.accountStore.RevokeAPIKey(r.Context(), sess.UserID, req.KeyID); err != nil {
		writePortalJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	writePortalJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) requirePortalSession(w http.ResponseWriter, r *http.Request) (billing.AuthSession, bool) {
	if !h.portalEnabled() {
		http.NotFound(w, r)
		return billing.AuthSession{}, false
	}
	token := h.sessionToken(r)
	if token == "" {
		writePortalJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "Not signed in",
			"code":  "unauthorized",
		})
		return billing.AuthSession{}, false
	}
	sess, err := h.accountStore.LookupAuthSession(r.Context(), token)
	if err != nil {
		if errors.Is(err, billing.ErrSessionNotFound) || errors.Is(err, billing.ErrSessionExpired) {
			h.clearSessionCookie(w)
			writePortalJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "Session expired or invalid",
				"code":  "unauthorized",
			})
			return billing.AuthSession{}, false
		}
		writePortalJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Session store unavailable"})
		return billing.AuthSession{}, false
	}
	return sess, true
}

func (h *Handler) issueSession(w http.ResponseWriter, r *http.Request, userID string) error {
	token, err := h.accountStore.CreateAuthSession(r.Context(), userID, h.portalSessionTTL)
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

func (h *Handler) githubCallbackURL() string {
	base := strings.TrimRight(strings.TrimSpace(h.portalBaseURL), "/")
	return base + "/portal/callback/github"
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

func writePortalJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func randomToken(nBytes int) (string, error) {
	raw := make([]byte, nBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

type githubUserInfo struct {
	Subject string
	Email   string
	Name    string
}

func exchangeGitHubCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (githubUserInfo, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://github.com/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return githubUserInfo{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return githubUserInfo{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return githubUserInfo{}, fmt.Errorf("github token exchange status %d", resp.StatusCode)
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return githubUserInfo{}, err
	}
	if tokenResp.AccessToken == "" {
		return githubUserInfo{}, fmt.Errorf("github token exchange: %s", tokenResp.Error)
	}

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return githubUserInfo{}, err
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userReq.Header.Set("Accept", "application/vnd.github+json")
	userReq.Header.Set("User-Agent", "TokenGuard")

	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		return githubUserInfo{}, err
	}
	defer userResp.Body.Close()
	userBody, _ := io.ReadAll(io.LimitReader(userResp.Body, 1<<20))
	if userResp.StatusCode >= 300 {
		return githubUserInfo{}, fmt.Errorf("github user status %d", userResp.StatusCode)
	}
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(userBody, &user); err != nil {
		return githubUserInfo{}, err
	}
	if user.ID == 0 {
		return githubUserInfo{}, errors.New("github user missing id")
	}

	email := strings.TrimSpace(user.Email)
	if email == "" {
		email, _ = fetchGitHubPrimaryEmail(ctx, tokenResp.AccessToken)
	}
	if email == "" {
		email = fmt.Sprintf("%d+%s@users.noreply.github.com", user.ID, user.Login)
	}
	name := strings.TrimSpace(user.Name)
	if name == "" {
		name = user.Login
	}
	return githubUserInfo{
		Subject: fmt.Sprintf("%d", user.ID),
		Email:   email,
		Name:    name,
	}, nil
}

func fetchGitHubPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "TokenGuard")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("github emails status %d", resp.StatusCode)
	}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}
	for _, e := range emails {
		if e.Primary && e.Verified && e.Email != "" {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified && e.Email != "" {
			return e.Email, nil
		}
	}
	return "", errors.New("no verified github email")
}
