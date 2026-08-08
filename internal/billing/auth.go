package billing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

var (
	ErrSessionNotFound = errors.New("auth session not found")
	ErrSessionExpired  = errors.New("auth session expired")
)

// AuthSession is a logged-in portal session (not an agent loop session_id).
type AuthSession struct {
	ID     string
	UserID string
}

// APIKeyMeta is safe to show in the portal (never includes plaintext).
type APIKeyMeta struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	KeyPrefix string `json:"key_prefix"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// AccountView is the signed-in user's portal snapshot.
type AccountView struct {
	UserID            string       `json:"user_id"`
	Email             string       `json:"email"`
	Name              string       `json:"name"`
	LimitMicroUSD     int64        `json:"limit_microusd"`
	SpentMicroUSD     int64        `json:"spent_microusd"`
	ReservedMicroUSD  int64        `json:"reserved_microusd"`
	AvailableMicroUSD int64        `json:"available_microusd"`
	BudgetUSD         float64      `json:"budget_usd"`
	SpentUSD          float64      `json:"spent_usd"`
	AvailableUSD      float64      `json:"available_usd"`
	Keys              []APIKeyMeta `json:"keys"`
	ActiveKeyCount    int          `json:"active_key_count"`
	Teams             []Team       `json:"teams"`
}

// EnsureOAuthUser finds or creates a user for an OAuth identity.
// Default budget applies only when a new user row is created.
func (s *Store) EnsureOAuthUser(ctx context.Context, provider, subject, email, name string, defaultLimitMicroUSD int64) (userID string, created bool, err error) {
	if s == nil || s.db == nil {
		return "", false, errors.New("billing store is nil")
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	subject = strings.TrimSpace(subject)
	email = strings.TrimSpace(strings.ToLower(email))
	name = strings.TrimSpace(name)
	if provider == "" || subject == "" {
		return "", false, errors.New("provider and subject are required")
	}
	if email == "" {
		email = fmt.Sprintf("%s_%s@users.tokenguard.local", provider, subject)
	}
	if defaultLimitMicroUSD <= 0 {
		defaultLimitMicroUSD = 1_000_000
	}

	var existingUserID string
	err = s.db.QueryRowContext(ctx, `
SELECT user_id FROM oauth_identities WHERE provider = ? AND subject = ?`, provider, subject).Scan(&existingUserID)
	if err == nil {
		return existingUserID, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, fmt.Errorf("lookup oauth identity: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()

	// Link to existing user by email when present.
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ? AND status = 'active'`, email).Scan(&existingUserID)
	switch {
	case err == nil:
		userID = existingUserID
		created = false
	case errors.Is(err, sql.ErrNoRows):
		userID, err = NewID("user")
		if err != nil {
			return "", false, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO users (id, email, name) VALUES (?, ?, ?)`, userID, email, name)
		if err != nil {
			return "", false, fmt.Errorf("insert user: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
INSERT INTO user_budgets (user_id, limit_microusd, period_start_at)
VALUES (?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, userID, defaultLimitMicroUSD)
		if err != nil {
			return "", false, fmt.Errorf("insert budget: %w", err)
		}
		created = true
	default:
		return "", false, fmt.Errorf("lookup user by email: %w", err)
	}

	identityID, err := NewID("oauth")
	if err != nil {
		return "", false, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO oauth_identities (id, user_id, provider, subject, email)
VALUES (?, ?, ?, ?, ?)`, identityID, userID, provider, subject, email)
	if err != nil {
		return "", false, fmt.Errorf("insert oauth identity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return userID, created, nil
}

// CreateAuthSession stores a hashed session token and returns the plaintext once.
func (s *Store) CreateAuthSession(ctx context.Context, userID string, ttl time.Duration) (plaintext string, err error) {
	if s == nil || s.db == nil {
		return "", errors.New("billing store is nil")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "", errors.New("user_id is required")
	}
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}

	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("read session token: %w", err)
	}
	plaintext = "tgs_" + hex.EncodeToString(raw[:])
	hash := HashAPIKey(plaintext)

	id, err := NewID("sess")
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().UTC().Add(ttl).Format(time.RFC3339Nano)
	_, err = s.db.ExecContext(ctx, `
INSERT INTO auth_sessions (id, user_id, token_hash, expires_at)
VALUES (?, ?, ?, ?)`, id, userID, hash, expiresAt)
	if err != nil {
		return "", fmt.Errorf("insert auth session: %w", err)
	}
	return plaintext, nil
}

// LookupAuthSession resolves an active, non-expired session token.
func (s *Store) LookupAuthSession(ctx context.Context, plaintext string) (AuthSession, error) {
	if s == nil || s.db == nil {
		return AuthSession{}, errors.New("billing store is nil")
	}
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return AuthSession{}, ErrSessionNotFound
	}
	hash := HashAPIKey(plaintext)

	var sess AuthSession
	var expiresAt string
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, expires_at
FROM auth_sessions
WHERE token_hash = ? AND revoked_at IS NULL`, hash).Scan(&sess.ID, &sess.UserID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AuthSession{}, ErrSessionNotFound
	}
	if err != nil {
		return AuthSession{}, err
	}

	exp, err := time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		exp, err = time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return AuthSession{}, fmt.Errorf("parse session expiry: %w", err)
		}
	}
	if time.Now().UTC().After(exp) {
		return AuthSession{}, ErrSessionExpired
	}
	return sess, nil
}

// RevokeAuthSession marks a session revoked by plaintext token.
func (s *Store) RevokeAuthSession(ctx context.Context, plaintext string) error {
	if s == nil || s.db == nil {
		return errors.New("billing store is nil")
	}
	plaintext = strings.TrimSpace(plaintext)
	if plaintext == "" {
		return nil
	}
	hash := HashAPIKey(plaintext)
	_, err := s.db.ExecContext(ctx, `
UPDATE auth_sessions
SET revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE token_hash = ? AND revoked_at IS NULL`, hash)
	return err
}

// GetAccountView returns budget + key metadata for the portal.
func (s *Store) GetAccountView(ctx context.Context, userID string) (AccountView, error) {
	if s == nil || s.db == nil {
		return AccountView{}, errors.New("billing store is nil")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return AccountView{}, errors.New("user_id is required")
	}

	var view AccountView
	err := s.db.QueryRowContext(ctx, `
SELECT u.id, u.email, IFNULL(u.name, ''),
       b.limit_microusd, b.spent_microusd, b.reserved_microusd
FROM users u
JOIN user_budgets b ON u.id = b.user_id
WHERE u.id = ? AND u.status = 'active'`, userID).Scan(
		&view.UserID, &view.Email, &view.Name,
		&view.LimitMicroUSD, &view.SpentMicroUSD, &view.ReservedMicroUSD,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AccountView{}, ErrBudgetNotFound
	}
	if err != nil {
		return AccountView{}, err
	}

	available := view.LimitMicroUSD - view.SpentMicroUSD - view.ReservedMicroUSD
	if available < 0 {
		available = 0
	}
	view.AvailableMicroUSD = available
	view.BudgetUSD = float64(view.LimitMicroUSD) / 1_000_000
	view.SpentUSD = float64(view.SpentMicroUSD) / 1_000_000
	view.AvailableUSD = float64(available) / 1_000_000

	keys, err := s.ListAPIKeyMeta(ctx, userID)
	if err != nil {
		return AccountView{}, err
	}
	if keys == nil {
		keys = []APIKeyMeta{}
	}
	view.Keys = keys
	for _, k := range keys {
		if k.Status == "active" {
			view.ActiveKeyCount++
		}
	}
	// Auto-accept any pending team invites for this email (member signed up after invite).
	if _, err := s.AcceptPendingInvitesForEmail(ctx, userID, view.Email); err != nil {
		log.Printf("accept pending team invites: %v", err)
	}
	teams, err := s.ListTeamsForUser(ctx, userID)
	if err != nil {
		return AccountView{}, err
	}
	if teams == nil {
		teams = []Team{}
	}
	view.Teams = teams
	return view, nil
}

// ListAPIKeyMeta lists key prefixes for a user (no plaintext).
func (s *Store) ListAPIKeyMeta(ctx context.Context, userID string) ([]APIKeyMeta, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, key_prefix, status, created_at
FROM api_keys
WHERE user_id = ?
ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []APIKeyMeta
	for rows.Next() {
		var k APIKeyMeta
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Status, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// CountActiveAPIKeys returns how many active keys a user has.
func (s *Store) CountActiveAPIKeys(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM api_keys WHERE user_id = ? AND status = 'active'`, userID).Scan(&n)
	return n, err
}

// RevokeAPIKey marks a user's key revoked.
func (s *Store) RevokeAPIKey(ctx context.Context, userID, keyID string) error {
	userID = strings.TrimSpace(userID)
	keyID = strings.TrimSpace(keyID)
	if userID == "" || keyID == "" {
		return errors.New("user_id and key id are required")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE api_keys
SET status = 'revoked',
    revoked_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ? AND user_id = ? AND status = 'active'`, keyID, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("api key not found or already revoked")
	}
	return nil
}
