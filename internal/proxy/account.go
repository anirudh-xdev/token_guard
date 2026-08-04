package proxy

import (
	"context"
	"time"

	"tokenguard/internal/billing"
)

// AccountStore backs the product portal (sign-in, sessions, my keys).
// Separate from BudgetStore so guard fakes stay unchanged.
type AccountStore interface {
	EnsureOAuthUser(ctx context.Context, provider, subject, email, name string, defaultLimitMicroUSD int64) (userID string, created bool, err error)
	CreateAuthSession(ctx context.Context, userID string, ttl time.Duration) (plaintext string, err error)
	LookupAuthSession(ctx context.Context, plaintext string) (billing.AuthSession, error)
	RevokeAuthSession(ctx context.Context, plaintext string) error
	GetAccountView(ctx context.Context, userID string) (billing.AccountView, error)
	CreateAPIKey(ctx context.Context, userID, name string) (string, string, error)
	CountActiveAPIKeys(ctx context.Context, userID string) (int, error)
	RevokeAPIKey(ctx context.Context, userID, keyID string) error
}

// Compile-time check: real Turso store implements AccountStore.
var _ AccountStore = (*billing.Store)(nil)
