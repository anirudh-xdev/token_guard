package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type PortalScope struct {
	Kind  string `json:"kind"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	Owner string `json:"owner,omitempty"`
	Days  int    `json:"days"`
}

type PortalBudgetSummary struct {
	LimitMicroUSD     int64 `json:"limit_microusd"`
	SpentMicroUSD     int64 `json:"spent_microusd"`
	ReservedMicroUSD  int64 `json:"reserved_microusd"`
	AvailableMicroUSD int64 `json:"available_microusd"`
}

type PortalUsageTotals struct {
	Requests       int64 `json:"requests"`
	Completed      int64 `json:"completed"`
	Blocked        int64 `json:"blocked"`
	ProviderErrors int64 `json:"provider_errors"`
	InputTokens    int64 `json:"input_tokens"`
	OutputTokens   int64 `json:"output_tokens"`
	CostMicroUSD   int64 `json:"cost_microusd"`
}

type PortalDailyUsage struct {
	Date         string `json:"date"`
	Requests     int64  `json:"requests"`
	Blocked      int64  `json:"blocked"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CostMicroUSD int64  `json:"cost_microusd"`
}

type PortalUsageBreakdown struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Requests     int64  `json:"requests"`
	Tokens       int64  `json:"tokens"`
	CostMicroUSD int64  `json:"cost_microusd"`
}

type PortalOverview struct {
	Scope              PortalScope            `json:"scope"`
	Budget             PortalBudgetSummary    `json:"budget"`
	Totals             PortalUsageTotals      `json:"totals"`
	Daily              []PortalDailyUsage     `json:"daily"`
	Breakdown          []PortalUsageBreakdown `json:"breakdown"`
	PendingInviteCount int64                  `json:"pending_invite_count,omitempty"`
}

// GetPortalOverview returns authoritative, bounded aggregates for one visible
// spend scope. Personal scope excludes explicitly team-attributed events.
func (s *Store) GetPortalOverview(ctx context.Context, userID, teamID string, days int) (PortalOverview, error) {
	if s == nil || s.db == nil {
		return PortalOverview{}, errors.New("billing store is nil")
	}
	userID = strings.TrimSpace(userID)
	teamID = strings.TrimSpace(teamID)
	if userID == "" {
		return PortalOverview{}, errors.New("user id is required")
	}
	if days < 7 || days > 90 {
		days = 30
	}

	overview := PortalOverview{
		Scope:     PortalScope{Kind: "personal", Name: "Personal", Role: "personal", Days: days},
		Daily:     []PortalDailyUsage{},
		Breakdown: []PortalUsageBreakdown{},
	}
	where := "user_id = ? AND team_id IS NULL"
	args := []any{userID}

	if teamID == "" {
		var budget Budget
		if err := s.db.QueryRowContext(ctx, `
SELECT user_id, limit_microusd, spent_microusd, reserved_microusd
FROM user_budgets WHERE user_id = ?`, userID).Scan(
			&budget.UserID, &budget.LimitMicroUSD, &budget.SpentMicroUSD, &budget.ReservedMicroUSD,
		); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return PortalOverview{}, ErrBudgetNotFound
			}
			return PortalOverview{}, err
		}
		overview.Budget = portalBudgetSummary(budget.LimitMicroUSD, budget.SpentMicroUSD, budget.ReservedMicroUSD)
	} else {
		var memberLimit, memberSpent, memberReserved int64
		var teamLimit, teamSpent, teamReserved int64
		err := s.db.QueryRowContext(ctx, `
SELECT t.name, m.role, IFNULL(ou.name, ou.email),
       m.cap_microusd, m.spent_microusd, m.reserved_microusd,
       t.limit_microusd, t.spent_microusd, t.reserved_microusd
FROM team_members m
JOIN teams t ON t.id = m.team_id
JOIN users ou ON ou.id = t.owner_user_id
WHERE m.team_id = ? AND m.user_id = ? AND m.status = 'active'`, teamID, userID).Scan(
			&overview.Scope.Name, &overview.Scope.Role, &overview.Scope.Owner,
			&memberLimit, &memberSpent, &memberReserved,
			&teamLimit, &teamSpent, &teamReserved,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return PortalOverview{}, ErrTeamNotFound
		}
		if err != nil {
			return PortalOverview{}, err
		}
		overview.Scope.Kind = "team"
		overview.Scope.ID = teamID
		if overview.Scope.Role == "owner" {
			overview.Budget = portalBudgetSummary(teamLimit, teamSpent, teamReserved)
			where = "team_id = ?"
			args = []any{teamID}
			if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM team_invites WHERE team_id = ? AND status = 'pending'`, teamID).Scan(&overview.PendingInviteCount); err != nil {
				return PortalOverview{}, err
			}
		} else {
			overview.Budget = portalBudgetSummary(memberLimit, memberSpent, memberReserved)
			where = "team_id = ? AND user_id = ?"
			args = []any{teamID, userID}
		}
	}

	window := fmt.Sprintf("-%d days", days)
	totalsQuery := `
SELECT COUNT(*),
       COALESCE(SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN status IN ('blocked_budget', 'blocked_loop') THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN status = 'provider_error' THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(input_tokens), 0),
       COALESCE(SUM(output_tokens), 0),
       COALESCE(SUM(actual_cost_microusd), 0)
FROM usage_events
WHERE ` + where + ` AND datetime(created_at) >= datetime('now', ?)`
	totalArgs := append(append([]any{}, args...), window)
	if err := s.db.QueryRowContext(ctx, totalsQuery, totalArgs...).Scan(
		&overview.Totals.Requests, &overview.Totals.Completed, &overview.Totals.Blocked,
		&overview.Totals.ProviderErrors, &overview.Totals.InputTokens,
		&overview.Totals.OutputTokens, &overview.Totals.CostMicroUSD,
	); err != nil {
		return PortalOverview{}, err
	}

	dailyQuery := `
SELECT substr(created_at, 1, 10), COUNT(*),
       COALESCE(SUM(CASE WHEN status IN ('blocked_budget', 'blocked_loop') THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
       COALESCE(SUM(actual_cost_microusd), 0)
FROM usage_events
WHERE ` + where + ` AND datetime(created_at) >= datetime('now', ?)
GROUP BY substr(created_at, 1, 10)
ORDER BY substr(created_at, 1, 10)`
	rows, err := s.db.QueryContext(ctx, dailyQuery, totalArgs...)
	if err != nil {
		return PortalOverview{}, err
	}
	for rows.Next() {
		var point PortalDailyUsage
		if err := rows.Scan(&point.Date, &point.Requests, &point.Blocked, &point.InputTokens, &point.OutputTokens, &point.CostMicroUSD); err != nil {
			rows.Close()
			return PortalOverview{}, err
		}
		overview.Daily = append(overview.Daily, point)
	}
	if err := rows.Close(); err != nil {
		return PortalOverview{}, err
	}

	breakdownQuery := `
SELECT provider, model, COUNT(*),
       COALESCE(SUM(input_tokens + output_tokens), 0),
       COALESCE(SUM(actual_cost_microusd), 0)
FROM usage_events
WHERE ` + where + ` AND datetime(created_at) >= datetime('now', ?)
GROUP BY provider, model
ORDER BY COALESCE(SUM(actual_cost_microusd), 0) DESC, COUNT(*) DESC
LIMIT 8`
	rows, err = s.db.QueryContext(ctx, breakdownQuery, totalArgs...)
	if err != nil {
		return PortalOverview{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item PortalUsageBreakdown
		if err := rows.Scan(&item.Provider, &item.Model, &item.Requests, &item.Tokens, &item.CostMicroUSD); err != nil {
			return PortalOverview{}, err
		}
		overview.Breakdown = append(overview.Breakdown, item)
	}
	return overview, rows.Err()
}

func portalBudgetSummary(limit, spent, reserved int64) PortalBudgetSummary {
	available := limit - spent - reserved
	if available < 0 {
		available = 0
	}
	return PortalBudgetSummary{
		LimitMicroUSD: limit, SpentMicroUSD: spent,
		ReservedMicroUSD: reserved, AvailableMicroUSD: available,
	}
}
