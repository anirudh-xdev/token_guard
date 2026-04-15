package billing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrAPIKeyNotFound = errors.New("active api key not found")
	ErrBudgetNotFound = errors.New("user budget not found")
)

type APIKey struct {
	ID        string
	UserID    string
	KeyPrefix string
}

type Budget struct {
	UserID           string
	LimitMicroUSD    int64
	SpentMicroUSD    int64
	ReservedMicroUSD int64
}

func (b Budget) AvailableMicroUSD() int64 {
	available := b.LimitMicroUSD - b.SpentMicroUSD - b.ReservedMicroUSD
	if available < 0 {
		return 0
	}
	return available
}

type UsageEvent struct {
	ID                    string
	UserID                string
	APIKeyID              string
	Provider              string
	Model                 string
	SessionID             string
	RequestID             string
	InputTokens           int64
	OutputTokens          int64
	EstimatedCostMicroUSD int64
	ActualCostMicroUSD    int64
	Status                string
}

func HashAPIKey(apiKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(apiKey)))
	return hex.EncodeToString(sum[:])
}

func NewUsageEventID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("read random usage id: %w", err)
	}
	return "evt_" + hex.EncodeToString(raw[:]), nil
}

func (s *Store) LookupAPIKey(ctx context.Context, plaintextKey string) (APIKey, error) {
	if s == nil || s.db == nil {
		return APIKey{}, errors.New("billing store is nil")
	}
	hash := HashAPIKey(plaintextKey)

	var key APIKey
	err := s.db.QueryRowContext(ctx, `
SELECT id, user_id, key_prefix
FROM api_keys
WHERE key_hash = ?
  AND status = 'active'
  AND (expires_at IS NULL OR expires_at > strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
LIMIT 1`, hash).Scan(&key.ID, &key.UserID, &key.KeyPrefix)
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrAPIKeyNotFound
	}
	if err != nil {
		return APIKey{}, fmt.Errorf("lookup api key: %w", err)
	}
	return key, nil
}

func (s *Store) GetUserBudget(ctx context.Context, userID string) (Budget, error) {
	if s == nil || s.db == nil {
		return Budget{}, errors.New("billing store is nil")
	}

	var budget Budget
	err := s.db.QueryRowContext(ctx, `
SELECT user_id, limit_microusd, spent_microusd, reserved_microusd
FROM user_budgets
WHERE user_id = ?
LIMIT 1`, strings.TrimSpace(userID)).Scan(
		&budget.UserID,
		&budget.LimitMicroUSD,
		&budget.SpentMicroUSD,
		&budget.ReservedMicroUSD,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Budget{}, ErrBudgetNotFound
	}
	if err != nil {
		return Budget{}, fmt.Errorf("get user budget: %w", err)
	}
	return budget, nil
}

func (s *Store) RecordUsage(ctx context.Context, event UsageEvent) error {
	if s == nil || s.db == nil {
		return errors.New("billing store is nil")
	}
	if event.ID == "" {
		id, err := NewUsageEventID()
		if err != nil {
			return err
		}
		event.ID = id
	}
	if event.Provider == "" {
		event.Provider = "openai"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin usage tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO usage_events (
  id, user_id, api_key_id, provider, model, session_id, request_id,
  input_tokens, output_tokens, estimated_cost_microusd, actual_cost_microusd, status
) VALUES (?, ?, NULLIF(?, ''), ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?)`,
		event.ID,
		event.UserID,
		event.APIKeyID,
		event.Provider,
		event.Model,
		event.SessionID,
		event.RequestID,
		event.InputTokens,
		event.OutputTokens,
		event.EstimatedCostMicroUSD,
		event.ActualCostMicroUSD,
		event.Status,
	); err != nil {
		return fmt.Errorf("insert usage event: %w", err)
	}

	if event.APIKeyID != "" {
		if _, err := tx.ExecContext(ctx, `
UPDATE api_keys
SET last_used_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?`, event.APIKeyID); err != nil {
			return fmt.Errorf("update api key last_used_at: %w", err)
		}
	}

	if event.Status == "completed" && event.ActualCostMicroUSD > 0 {
		if _, err := tx.ExecContext(ctx, `
UPDATE user_budgets
SET spent_microusd = spent_microusd + ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE user_id = ?`, event.ActualCostMicroUSD, event.UserID); err != nil {
			return fmt.Errorf("update user budget spend: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit usage tx: %w", err)
	}
	return nil
}

func NowRFC3339Millis() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}
