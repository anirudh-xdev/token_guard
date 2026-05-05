package billing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

type UserBudgetView struct {
	UserID        string `json:"user_id"`
	Email         string `json:"email"`
	Name          string `json:"name"`
	LimitMicroUSD int64  `json:"limit_microusd"`
	SpentMicroUSD int64  `json:"spent_microusd"`
}

func NewID(prefix string) (string, error) {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("read random id: %w", err)
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}

func (s *Store) CreateUser(ctx context.Context, email, name string) (string, error) {
	id, err := NewID("user")
	if err != nil {
		return "", err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO users (id, email, name) VALUES (?, ?, ?)`, id, email, name)
	if err != nil {
		return "", fmt.Errorf("insert user: %w", err)
	}

	// Create default budget ($1.00)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO user_budgets (user_id, limit_microusd, period_start_at) 
		VALUES (?, 1000000, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, id)
	if err != nil {
		return "", fmt.Errorf("insert budget: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, userID, name string) (string, string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", "", err
	}
	plaintext := "tg_" + hex.EncodeToString(raw[:])
	hash := HashAPIKey(plaintext)
	prefix := plaintext[:6]

	id, err := NewID("key")
	if err != nil {
		return "", "", err
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, user_id, name, key_prefix, key_hash)
		VALUES (?, ?, ?, ?, ?)`, id, userID, name, prefix, hash)
	if err != nil {
		return "", "", fmt.Errorf("insert api key: %w", err)
	}

	return id, plaintext, nil
}

func (s *Store) ListUsers(ctx context.Context) ([]UserBudgetView, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT u.id, u.email, IFNULL(u.name, ''), b.limit_microusd, b.spent_microusd
		FROM users u
		JOIN user_budgets b ON u.id = b.user_id
		ORDER BY u.created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserBudgetView
	for rows.Next() {
		var u UserBudgetView
		if err := rows.Scan(&u.UserID, &u.Email, &u.Name, &u.LimitMicroUSD, &u.SpentMicroUSD); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (s *Store) ListRecentUsage(ctx context.Context, limit int) ([]UsageEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, IFNULL(api_key_id, ''), provider, model, IFNULL(session_id, ''), input_tokens, output_tokens, estimated_cost_microusd, actual_cost_microusd, status
		FROM usage_events
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []UsageEvent
	for rows.Next() {
		var e UsageEvent
		if err := rows.Scan(&e.ID, &e.UserID, &e.APIKeyID, &e.Provider, &e.Model, &e.SessionID, &e.InputTokens, &e.OutputTokens, &e.EstimatedCostMicroUSD, &e.ActualCostMicroUSD, &e.Status); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, nil
}
