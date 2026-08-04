package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrTeamNotFound       = errors.New("team not found")
	ErrNotTeamOwner       = errors.New("not team owner")
	ErrTeamMemberExists   = errors.New("user is already a team member")
	ErrTeamMemberNotFound = errors.New("team member not found")
)

type Team struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	OwnerUserID      string  `json:"owner_user_id"`
	LimitMicroUSD    int64   `json:"limit_microusd"`
	SpentMicroUSD    int64   `json:"spent_microusd"`
	ReservedMicroUSD int64   `json:"reserved_microusd"`
	AvailableMicroUSD int64  `json:"available_microusd"`
	BudgetUSD        float64 `json:"budget_usd"`
	SpentUSD         float64 `json:"spent_usd"`
	AvailableUSD     float64 `json:"available_usd"`
	MyRole           string  `json:"my_role,omitempty"`
	MyCapMicroUSD    int64   `json:"my_cap_microusd,omitempty"`
	MyCapUSD         float64 `json:"my_cap_usd,omitempty"`
	MySpentMicroUSD  int64   `json:"my_spent_microusd,omitempty"`
	MySpentUSD       float64 `json:"my_spent_usd,omitempty"`
}

type TeamMember struct {
	UserID           string  `json:"user_id"`
	Email            string  `json:"email"`
	Name             string  `json:"name"`
	Role             string  `json:"role"`
	CapMicroUSD      int64   `json:"cap_microusd"`
	SpentMicroUSD    int64   `json:"spent_microusd"`
	ReservedMicroUSD int64   `json:"reserved_microusd"`
	AvailableMicroUSD int64  `json:"available_microusd"`
	CapUSD           float64 `json:"cap_usd"`
	SpentUSD         float64 `json:"spent_usd"`
	AvailableUSD     float64 `json:"available_usd"`
}

type teamSpendScope struct {
	TeamID           string
	MemberCap        int64
	MemberSpent      int64
	MemberReserved   int64
	TeamLimit        int64
	TeamSpent        int64
	TeamReserved     int64
}

func (s *Store) CreateTeam(ctx context.Context, ownerUserID, name string, limitMicroUSD int64) (Team, error) {
	if s == nil || s.db == nil {
		return Team{}, errors.New("billing store is nil")
	}
	ownerUserID = strings.TrimSpace(ownerUserID)
	name = strings.TrimSpace(name)
	if ownerUserID == "" || name == "" {
		return Team{}, errors.New("owner and name are required")
	}
	if limitMicroUSD <= 0 {
		limitMicroUSD = 1_000_000
	}

	teamID, err := NewID("team")
	if err != nil {
		return Team{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Team{}, err
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
INSERT INTO teams (id, name, owner_user_id, limit_microusd)
VALUES (?, ?, ?, ?)`, teamID, name, ownerUserID, limitMicroUSD)
	if err != nil {
		return Team{}, fmt.Errorf("insert team: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO team_members (team_id, user_id, role, cap_microusd)
VALUES (?, ?, 'owner', ?)`, teamID, ownerUserID, limitMicroUSD)
	if err != nil {
		return Team{}, fmt.Errorf("insert owner membership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Team{}, err
	}
	return s.GetTeamForUser(ctx, teamID, ownerUserID)
}

func (s *Store) ListTeamsForUser(ctx context.Context, userID string) ([]Team, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id, t.name, t.owner_user_id, t.limit_microusd, t.spent_microusd, t.reserved_microusd,
       m.role, m.cap_microusd, m.spent_microusd
FROM teams t
JOIN team_members m ON m.team_id = t.id
WHERE m.user_id = ? AND m.status = 'active'
ORDER BY t.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Team
	for rows.Next() {
		var t Team
		if err := rows.Scan(
			&t.ID, &t.Name, &t.OwnerUserID, &t.LimitMicroUSD, &t.SpentMicroUSD, &t.ReservedMicroUSD,
			&t.MyRole, &t.MyCapMicroUSD, &t.MySpentMicroUSD,
		); err != nil {
			return nil, err
		}
		fillTeamMoney(&t)
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTeamForUser(ctx context.Context, teamID, userID string) (Team, error) {
	var t Team
	err := s.db.QueryRowContext(ctx, `
SELECT t.id, t.name, t.owner_user_id, t.limit_microusd, t.spent_microusd, t.reserved_microusd,
       m.role, m.cap_microusd, m.spent_microusd
FROM teams t
JOIN team_members m ON m.team_id = t.id
WHERE t.id = ? AND m.user_id = ? AND m.status = 'active'`, teamID, userID).Scan(
		&t.ID, &t.Name, &t.OwnerUserID, &t.LimitMicroUSD, &t.SpentMicroUSD, &t.ReservedMicroUSD,
		&t.MyRole, &t.MyCapMicroUSD, &t.MySpentMicroUSD,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Team{}, ErrTeamNotFound
	}
	if err != nil {
		return Team{}, err
	}
	fillTeamMoney(&t)
	return t, nil
}

func (s *Store) UpdateTeamBudget(ctx context.Context, ownerUserID, teamID string, limitMicroUSD int64) (Team, error) {
	if limitMicroUSD < 0 {
		return Team{}, errors.New("limit cannot be negative")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE teams
SET limit_microusd = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ? AND owner_user_id = ?`, limitMicroUSD, teamID, ownerUserID)
	if err != nil {
		return Team{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Team{}, ErrNotTeamOwner
	}
	// Keep owner cap aligned with pool by default (owner can still edit member caps separately).
	_, _ = s.db.ExecContext(ctx, `
UPDATE team_members
SET cap_microusd = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE team_id = ? AND user_id = ? AND role = 'owner'`, limitMicroUSD, teamID, ownerUserID)
	return s.GetTeamForUser(ctx, teamID, ownerUserID)
}

func (s *Store) AddTeamMemberByEmail(ctx context.Context, ownerUserID, teamID, email string, capMicroUSD int64) (TeamMember, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return TeamMember{}, errors.New("email is required")
	}
	if capMicroUSD < 0 {
		return TeamMember{}, errors.New("cap cannot be negative")
	}

	var owner string
	err := s.db.QueryRowContext(ctx, `SELECT owner_user_id FROM teams WHERE id = ?`, teamID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return TeamMember{}, ErrTeamNotFound
	}
	if err != nil {
		return TeamMember{}, err
	}
	if owner != ownerUserID {
		return TeamMember{}, ErrNotTeamOwner
	}

	var memberUserID, name string
	err = s.db.QueryRowContext(ctx, `
SELECT id, IFNULL(name, '') FROM users WHERE email = ? AND status = 'active'`, email).Scan(&memberUserID, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return TeamMember{}, fmt.Errorf("no TokenGuard account for email %s — they must sign in at /portal first", email)
	}
	if err != nil {
		return TeamMember{}, err
	}

	_, err = s.db.ExecContext(ctx, `
INSERT INTO team_members (team_id, user_id, role, cap_microusd)
VALUES (?, ?, 'member', ?)`, teamID, memberUserID, capMicroUSD)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return TeamMember{}, ErrTeamMemberExists
		}
		return TeamMember{}, err
	}
	return s.getTeamMember(ctx, teamID, memberUserID)
}

func (s *Store) UpdateTeamMemberCap(ctx context.Context, ownerUserID, teamID, memberUserID string, capMicroUSD int64) (TeamMember, error) {
	if capMicroUSD < 0 {
		return TeamMember{}, errors.New("cap cannot be negative")
	}
	var owner string
	err := s.db.QueryRowContext(ctx, `SELECT owner_user_id FROM teams WHERE id = ?`, teamID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return TeamMember{}, ErrTeamNotFound
	}
	if err != nil {
		return TeamMember{}, err
	}
	if owner != ownerUserID {
		return TeamMember{}, ErrNotTeamOwner
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE team_members
SET cap_microusd = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE team_id = ? AND user_id = ? AND status = 'active'`, capMicroUSD, teamID, memberUserID)
	if err != nil {
		return TeamMember{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return TeamMember{}, ErrTeamMemberNotFound
	}
	return s.getTeamMember(ctx, teamID, memberUserID)
}

func (s *Store) RemoveTeamMember(ctx context.Context, ownerUserID, teamID, memberUserID string) error {
	var owner string
	err := s.db.QueryRowContext(ctx, `SELECT owner_user_id FROM teams WHERE id = ?`, teamID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTeamNotFound
	}
	if err != nil {
		return err
	}
	if owner != ownerUserID {
		return ErrNotTeamOwner
	}
	if memberUserID == ownerUserID {
		return errors.New("cannot remove team owner")
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE team_members
SET status = 'removed',
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE team_id = ? AND user_id = ? AND status = 'active'`, teamID, memberUserID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrTeamMemberNotFound
	}
	return nil
}

func (s *Store) ListTeamMembers(ctx context.Context, requesterUserID, teamID string) ([]TeamMember, error) {
	// Must be an active member to list.
	var role string
	err := s.db.QueryRowContext(ctx, `
SELECT role FROM team_members WHERE team_id = ? AND user_id = ? AND status = 'active'`, teamID, requesterUserID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTeamNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT m.user_id, u.email, IFNULL(u.name, ''), m.role, m.cap_microusd, m.spent_microusd, m.reserved_microusd
FROM team_members m
JOIN users u ON u.id = m.user_id
WHERE m.team_id = ? AND m.status = 'active'
ORDER BY CASE m.role WHEN 'owner' THEN 0 ELSE 1 END, u.email`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TeamMember
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.CapMicroUSD, &m.SpentMicroUSD, &m.ReservedMicroUSD); err != nil {
			return nil, err
		}
		fillMemberMoney(&m)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) getTeamMember(ctx context.Context, teamID, userID string) (TeamMember, error) {
	var m TeamMember
	err := s.db.QueryRowContext(ctx, `
SELECT m.user_id, u.email, IFNULL(u.name, ''), m.role, m.cap_microusd, m.spent_microusd, m.reserved_microusd
FROM team_members m
JOIN users u ON u.id = m.user_id
WHERE m.team_id = ? AND m.user_id = ? AND m.status = 'active'`, teamID, userID).Scan(
		&m.UserID, &m.Email, &m.Name, &m.Role, &m.CapMicroUSD, &m.SpentMicroUSD, &m.ReservedMicroUSD,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return TeamMember{}, ErrTeamMemberNotFound
	}
	if err != nil {
		return TeamMember{}, err
	}
	fillMemberMoney(&m)
	return m, nil
}

func (s *Store) lookupTeamSpendScope(ctx context.Context, q budgetScanner, userID string) (teamSpendScope, bool, error) {
	var scope teamSpendScope
	err := q.QueryRowContext(ctx, `
SELECT m.team_id, m.cap_microusd, m.spent_microusd, m.reserved_microusd,
       t.limit_microusd, t.spent_microusd, t.reserved_microusd
FROM team_members m
JOIN teams t ON t.id = m.team_id
WHERE m.user_id = ? AND m.status = 'active'
ORDER BY CASE m.role WHEN 'owner' THEN 0 ELSE 1 END, t.created_at ASC
LIMIT 1`, userID).Scan(
		&scope.TeamID, &scope.MemberCap, &scope.MemberSpent, &scope.MemberReserved,
		&scope.TeamLimit, &scope.TeamSpent, &scope.TeamReserved,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return teamSpendScope{}, false, nil
	}
	if err != nil {
		return teamSpendScope{}, false, err
	}
	return scope, true, nil
}

func fillTeamMoney(t *Team) {
	avail := t.LimitMicroUSD - t.SpentMicroUSD - t.ReservedMicroUSD
	if avail < 0 {
		avail = 0
	}
	t.AvailableMicroUSD = avail
	t.BudgetUSD = float64(t.LimitMicroUSD) / 1_000_000
	t.SpentUSD = float64(t.SpentMicroUSD) / 1_000_000
	t.AvailableUSD = float64(avail) / 1_000_000
	t.MyCapUSD = float64(t.MyCapMicroUSD) / 1_000_000
	t.MySpentUSD = float64(t.MySpentMicroUSD) / 1_000_000
}

func fillMemberMoney(m *TeamMember) {
	avail := m.CapMicroUSD - m.SpentMicroUSD - m.ReservedMicroUSD
	if avail < 0 {
		avail = 0
	}
	m.AvailableMicroUSD = avail
	m.CapUSD = float64(m.CapMicroUSD) / 1_000_000
	m.SpentUSD = float64(m.SpentMicroUSD) / 1_000_000
	m.AvailableUSD = float64(avail) / 1_000_000
}
