package proxy

import (
	"context"
	"time"

	"tokenguard/internal/billing"
)

// AccountStore backs the product portal (sign-in, sessions, my keys, teams).
type AccountStore interface {
	EnsureOAuthUser(ctx context.Context, provider, subject, email, name string, defaultLimitMicroUSD int64) (userID string, created bool, err error)
	CreateAuthSession(ctx context.Context, userID string, ttl time.Duration) (plaintext string, err error)
	LookupAuthSession(ctx context.Context, plaintext string) (billing.AuthSession, error)
	RevokeAuthSession(ctx context.Context, plaintext string) error
	GetAccountView(ctx context.Context, userID string) (billing.AccountView, error)
	CreateAPIKey(ctx context.Context, userID, name string) (string, string, error)
	CountActiveAPIKeys(ctx context.Context, userID string) (int, error)
	RevokeAPIKey(ctx context.Context, userID, keyID string) error
	CreateTeam(ctx context.Context, ownerUserID, name string, limitMicroUSD int64) (billing.Team, error)
	ListTeamsForUser(ctx context.Context, userID string) ([]billing.Team, error)
	GetTeamForUser(ctx context.Context, teamID, userID string) (billing.Team, error)
	UpdateTeamBudget(ctx context.Context, ownerUserID, teamID string, limitMicroUSD int64) (billing.Team, error)
	AddTeamMemberByEmail(ctx context.Context, ownerUserID, teamID, email string, capMicroUSD int64) (billing.TeamMember, error)
	AddTeamMemberOrInvite(ctx context.Context, ownerUserID, teamID, email string, capMicroUSD int64) (billing.AddMemberResult, error)
	UpdateTeamMemberCap(ctx context.Context, ownerUserID, teamID, memberUserID string, capMicroUSD int64) (billing.TeamMember, error)
	RemoveTeamMember(ctx context.Context, ownerUserID, teamID, memberUserID string) error
	ListTeamMembers(ctx context.Context, requesterUserID, teamID string) ([]billing.TeamMember, error)
	ListPendingInvitesForTeam(ctx context.Context, ownerUserID, teamID string) ([]billing.TeamInvite, error)
	ListPortalUsage(ctx context.Context, userID, teamID string, limit int) ([]billing.UsageEvent, error)
}

var _ AccountStore = (*billing.Store)(nil)
