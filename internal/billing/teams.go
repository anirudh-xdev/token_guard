package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type spendTeamIDKey struct{}

// WithSpendTeamID selects which team pool/cap to charge for this request.
func WithSpendTeamID(ctx context.Context, teamID string) context.Context {
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return ctx
	}
	return context.WithValue(ctx, spendTeamIDKey{}, teamID)
}

// SpendTeamIDFromContext returns the preferred team id, if any.
func SpendTeamIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(spendTeamIDKey{}).(string)
	return strings.TrimSpace(v)
}

var (
	ErrTeamNotFound       = errors.New("team not found")
	ErrNotTeamOwner       = errors.New("not team owner")
	ErrTeamMemberExists   = errors.New("user is already a team member")
	ErrTeamMemberNotFound = errors.New("team member not found")
	ErrCapExceedsPool     = errors.New("member cap cannot exceed team pool budget")
	ErrTeamInvitePending  = errors.New("invite already pending for this email")
)

type Team struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	OwnerUserID       string  `json:"owner_user_id"`
	OwnerEmail        string  `json:"owner_email,omitempty"`
	OwnerName         string  `json:"owner_name,omitempty"`
	LimitMicroUSD     int64   `json:"limit_microusd"`
	SpentMicroUSD     int64   `json:"spent_microusd"`
	ReservedMicroUSD  int64   `json:"reserved_microusd"`
	AvailableMicroUSD int64   `json:"available_microusd"`
	BudgetUSD         float64 `json:"budget_usd"`
	SpentUSD          float64 `json:"spent_usd"`
	AvailableUSD      float64 `json:"available_usd"`
	MyRole            string  `json:"my_role,omitempty"`
	MyCapMicroUSD     int64   `json:"my_cap_microusd,omitempty"`
	MyCapUSD          float64 `json:"my_cap_usd,omitempty"`
	MySpentMicroUSD   int64   `json:"my_spent_microusd,omitempty"`
	MySpentUSD        float64 `json:"my_spent_usd,omitempty"`
	MyAvailableUSD    float64 `json:"my_available_usd,omitempty"`
	InvitedByUserID   string  `json:"invited_by_user_id,omitempty"`
	InvitedByEmail    string  `json:"invited_by_email,omitempty"`
	InvitedByName     string  `json:"invited_by_name,omitempty"`
	InvitedAt         string  `json:"invited_at,omitempty"`
}

type TeamMember struct {
	UserID            string  `json:"user_id"`
	Email             string  `json:"email"`
	Name              string  `json:"name"`
	Role              string  `json:"role"`
	CapMicroUSD       int64   `json:"cap_microusd"`
	SpentMicroUSD     int64   `json:"spent_microusd"`
	ReservedMicroUSD  int64   `json:"reserved_microusd"`
	AvailableMicroUSD int64   `json:"available_microusd"`
	CapUSD            float64 `json:"cap_usd"`
	SpentUSD          float64 `json:"spent_usd"`
	AvailableUSD      float64 `json:"available_usd"`
	InvitedByEmail    string  `json:"invited_by_email,omitempty"`
	InvitedAt         string  `json:"invited_at,omitempty"`
}

type TeamInvite struct {
	ID             string  `json:"id"`
	TeamID         string  `json:"team_id"`
	TeamName       string  `json:"team_name"`
	Email          string  `json:"email"`
	CapMicroUSD    int64   `json:"cap_microusd"`
	CapUSD         float64 `json:"cap_usd"`
	InvitedByEmail string  `json:"invited_by_email"`
	InvitedByName  string  `json:"invited_by_name"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
}

type teamSpendScope struct {
	TeamID         string
	MemberCap      int64
	MemberSpent    int64
	MemberReserved int64
	TeamLimit      int64
	TeamSpent      int64
	TeamReserved   int64
}

type AddMemberResult struct {
	Member       *TeamMember `json:"member,omitempty"`
	Invite       *TeamInvite `json:"invite,omitempty"`
	PendingInvite bool       `json:"pending_invite"`
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
INSERT INTO team_members (team_id, user_id, role, cap_microusd, invited_by_user_id, invited_at)
VALUES (?, ?, 'owner', ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`, teamID, ownerUserID, limitMicroUSD, ownerUserID)
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
       m.role, m.cap_microusd, m.spent_microusd, m.reserved_microusd,
       IFNULL(m.invited_by_user_id, ''), IFNULL(m.invited_at, ''),
       IFNULL(ou.email, ''), IFNULL(ou.name, ''),
       IFNULL(iu.email, ''), IFNULL(iu.name, '')
FROM teams t
JOIN team_members m ON m.team_id = t.id
JOIN users ou ON ou.id = t.owner_user_id
LEFT JOIN users iu ON iu.id = m.invited_by_user_id
WHERE m.user_id = ? AND m.status = 'active'
ORDER BY t.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Team
	for rows.Next() {
		var t Team
		var myReserved int64
		if err := rows.Scan(
			&t.ID, &t.Name, &t.OwnerUserID, &t.LimitMicroUSD, &t.SpentMicroUSD, &t.ReservedMicroUSD,
			&t.MyRole, &t.MyCapMicroUSD, &t.MySpentMicroUSD, &myReserved,
			&t.InvitedByUserID, &t.InvitedAt,
			&t.OwnerEmail, &t.OwnerName,
			&t.InvitedByEmail, &t.InvitedByName,
		); err != nil {
			return nil, err
		}
		fillTeamMoney(&t)
		myAvail := t.MyCapMicroUSD - t.MySpentMicroUSD - myReserved
		if myAvail < 0 {
			myAvail = 0
		}
		t.MyAvailableUSD = float64(myAvail) / 1_000_000
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) GetTeamForUser(ctx context.Context, teamID, userID string) (Team, error) {
	teams, err := s.ListTeamsForUser(ctx, userID)
	if err != nil {
		return Team{}, err
	}
	for _, t := range teams {
		if t.ID == teamID {
			return t, nil
		}
	}
	return Team{}, ErrTeamNotFound
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
	_, _ = s.db.ExecContext(ctx, `
UPDATE team_members
SET cap_microusd = ?,
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE team_id = ? AND user_id = ? AND role = 'owner'`, limitMicroUSD, teamID, ownerUserID)
	return s.GetTeamForUser(ctx, teamID, ownerUserID)
}

func (s *Store) AddTeamMemberByEmail(ctx context.Context, ownerUserID, teamID, email string, capMicroUSD int64) (TeamMember, error) {
	res, err := s.AddTeamMemberOrInvite(ctx, ownerUserID, teamID, email, capMicroUSD)
	if err != nil {
		return TeamMember{}, err
	}
	if res.PendingInvite {
		return TeamMember{}, fmt.Errorf("invite created for %s — they will join after signing in at /portal", email)
	}
	if res.Member == nil {
		return TeamMember{}, errors.New("member not created")
	}
	return *res.Member, nil
}

// AddTeamMemberOrInvite adds an existing user, or creates a pending invite if they have no account yet.
func (s *Store) AddTeamMemberOrInvite(ctx context.Context, ownerUserID, teamID, email string, capMicroUSD int64) (AddMemberResult, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return AddMemberResult{}, errors.New("email is required")
	}
	if capMicroUSD < 0 {
		return AddMemberResult{}, errors.New("cap cannot be negative")
	}

	var owner string
	var teamLimit int64
	var teamName string
	err := s.db.QueryRowContext(ctx, `SELECT owner_user_id, limit_microusd, name FROM teams WHERE id = ?`, teamID).
		Scan(&owner, &teamLimit, &teamName)
	if errors.Is(err, sql.ErrNoRows) {
		return AddMemberResult{}, ErrTeamNotFound
	}
	if err != nil {
		return AddMemberResult{}, err
	}
	if owner != ownerUserID {
		return AddMemberResult{}, ErrNotTeamOwner
	}
	if capMicroUSD > teamLimit {
		return AddMemberResult{}, ErrCapExceedsPool
	}

	var memberUserID, name string
	err = s.db.QueryRowContext(ctx, `
SELECT id, IFNULL(name, '') FROM users WHERE email = ? AND status = 'active'`, email).Scan(&memberUserID, &name)
	if errors.Is(err, sql.ErrNoRows) {
		invite, ierr := s.createPendingInvite(ctx, ownerUserID, teamID, teamName, email, capMicroUSD)
		if ierr != nil {
			return AddMemberResult{}, ierr
		}
		return AddMemberResult{Invite: &invite, PendingInvite: true}, nil
	}
	if err != nil {
		return AddMemberResult{}, err
	}
	if memberUserID == ownerUserID {
		return AddMemberResult{}, errors.New("owner is already on the team")
	}

	member, err := s.upsertActiveMember(ctx, teamID, memberUserID, ownerUserID, capMicroUSD)
	if err != nil {
		return AddMemberResult{}, err
	}
	return AddMemberResult{Member: &member}, nil
}

func (s *Store) createPendingInvite(ctx context.Context, ownerUserID, teamID, teamName, email string, capMicroUSD int64) (TeamInvite, error) {
	var existing string
	err := s.db.QueryRowContext(ctx, `
SELECT id FROM team_invites WHERE team_id = ? AND email = ? AND status = 'pending'`, teamID, email).Scan(&existing)
	if err == nil {
		return TeamInvite{}, ErrTeamInvitePending
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return TeamInvite{}, err
	}
	id, err := NewID("inv")
	if err != nil {
		return TeamInvite{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO team_invites (id, team_id, email, cap_microusd, invited_by_user_id, status)
VALUES (?, ?, ?, ?, ?, 'pending')`, id, teamID, email, capMicroUSD, ownerUserID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return TeamInvite{}, ErrTeamInvitePending
		}
		return TeamInvite{}, err
	}
	var byEmail, byName string
	_ = s.db.QueryRowContext(ctx, `SELECT email, IFNULL(name, '') FROM users WHERE id = ?`, ownerUserID).Scan(&byEmail, &byName)
	return TeamInvite{
		ID: id, TeamID: teamID, TeamName: teamName, Email: email,
		CapMicroUSD: capMicroUSD, CapUSD: float64(capMicroUSD) / 1e6,
		InvitedByEmail: byEmail, InvitedByName: byName, Status: "pending",
	}, nil
}

func (s *Store) upsertActiveMember(ctx context.Context, teamID, memberUserID, invitedBy string, capMicroUSD int64) (TeamMember, error) {
	var existingStatus string
	err := s.db.QueryRowContext(ctx, `
SELECT status FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, memberUserID).Scan(&existingStatus)
	if err == nil {
		if existingStatus == "active" {
			return TeamMember{}, ErrTeamMemberExists
		}
		_, err = s.db.ExecContext(ctx, `
UPDATE team_members
SET status = 'active',
    role = 'member',
    cap_microusd = ?,
    spent_microusd = 0,
    reserved_microusd = 0,
    invited_by_user_id = ?,
    invited_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE team_id = ? AND user_id = ?`, capMicroUSD, invitedBy, teamID, memberUserID)
		if err != nil {
			return TeamMember{}, err
		}
		return s.getTeamMember(ctx, teamID, memberUserID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TeamMember{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO team_members (team_id, user_id, role, cap_microusd, invited_by_user_id, invited_at)
VALUES (?, ?, 'member', ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))`,
		teamID, memberUserID, capMicroUSD, invitedBy)
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
	var teamLimit int64
	err := s.db.QueryRowContext(ctx, `SELECT owner_user_id, limit_microusd FROM teams WHERE id = ?`, teamID).Scan(&owner, &teamLimit)
	if errors.Is(err, sql.ErrNoRows) {
		return TeamMember{}, ErrTeamNotFound
	}
	if err != nil {
		return TeamMember{}, err
	}
	if owner != ownerUserID {
		return TeamMember{}, ErrNotTeamOwner
	}
	if capMicroUSD > teamLimit {
		return TeamMember{}, ErrCapExceedsPool
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
SELECT m.user_id, u.email, IFNULL(u.name, ''), m.role, m.cap_microusd, m.spent_microusd, m.reserved_microusd,
       IFNULL(m.invited_at, ''), IFNULL(iu.email, '')
FROM team_members m
JOIN users u ON u.id = m.user_id
LEFT JOIN users iu ON iu.id = m.invited_by_user_id
WHERE m.team_id = ? AND m.status = 'active'
ORDER BY CASE m.role WHEN 'owner' THEN 0 ELSE 1 END, u.email`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TeamMember
	for rows.Next() {
		var m TeamMember
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.Role, &m.CapMicroUSD, &m.SpentMicroUSD, &m.ReservedMicroUSD, &m.InvitedAt, &m.InvitedByEmail); err != nil {
			return nil, err
		}
		fillMemberMoney(&m)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ListPendingInvitesForTeam(ctx context.Context, ownerUserID, teamID string) ([]TeamInvite, error) {
	var owner string
	err := s.db.QueryRowContext(ctx, `SELECT owner_user_id FROM teams WHERE id = ?`, teamID).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrTeamNotFound
	}
	if err != nil {
		return nil, err
	}
	if owner != ownerUserID {
		return nil, ErrNotTeamOwner
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT i.id, i.team_id, t.name, i.email, i.cap_microusd, i.status, i.created_at,
       u.email, IFNULL(u.name, '')
FROM team_invites i
JOIN teams t ON t.id = i.team_id
JOIN users u ON u.id = i.invited_by_user_id
WHERE i.team_id = ? AND i.status = 'pending'
ORDER BY i.created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TeamInvite
	for rows.Next() {
		var inv TeamInvite
		if err := rows.Scan(&inv.ID, &inv.TeamID, &inv.TeamName, &inv.Email, &inv.CapMicroUSD, &inv.Status, &inv.CreatedAt, &inv.InvitedByEmail, &inv.InvitedByName); err != nil {
			return nil, err
		}
		inv.CapUSD = float64(inv.CapMicroUSD) / 1e6
		out = append(out, inv)
	}
	return out, rows.Err()
}

// AcceptPendingInvitesForEmail activates any pending invites for this email (call after sign-in).
func (s *Store) AcceptPendingInvitesForEmail(ctx context.Context, userID, email string) (int, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if userID == "" || email == "" {
		return 0, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, team_id, cap_microusd, invited_by_user_id
FROM team_invites
WHERE email = ? AND status = 'pending'`, email)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type pend struct {
		id, teamID, invitedBy string
		cap                   int64
	}
	var pending []pend
	for rows.Next() {
		var p pend
		if err := rows.Scan(&p.id, &p.teamID, &p.cap, &p.invitedBy); err != nil {
			return 0, err
		}
		pending = append(pending, p)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	accepted := 0
	for _, p := range pending {
		if _, err := s.upsertActiveMember(ctx, p.teamID, userID, p.invitedBy, p.cap); err != nil {
			if errors.Is(err, ErrTeamMemberExists) {
				// Already a member — mark invite accepted anyway.
			} else {
				continue
			}
		}
		_, _ = s.db.ExecContext(ctx, `
UPDATE team_invites
SET status = 'accepted',
    updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE id = ?`, p.id)
		accepted++
	}
	return accepted, nil
}

func (s *Store) getTeamMember(ctx context.Context, teamID, userID string) (TeamMember, error) {
	var m TeamMember
	err := s.db.QueryRowContext(ctx, `
SELECT m.user_id, u.email, IFNULL(u.name, ''), m.role, m.cap_microusd, m.spent_microusd, m.reserved_microusd,
       IFNULL(m.invited_at, ''), IFNULL(iu.email, '')
FROM team_members m
JOIN users u ON u.id = m.user_id
LEFT JOIN users iu ON iu.id = m.invited_by_user_id
WHERE m.team_id = ? AND m.user_id = ? AND m.status = 'active'`, teamID, userID).Scan(
		&m.UserID, &m.Email, &m.Name, &m.Role, &m.CapMicroUSD, &m.SpentMicroUSD, &m.ReservedMicroUSD, &m.InvitedAt, &m.InvitedByEmail,
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
	preferred := SpendTeamIDFromContext(ctx)
	var scope teamSpendScope
	var err error
	if preferred != "" {
		err = q.QueryRowContext(ctx, `
SELECT m.team_id, m.cap_microusd, m.spent_microusd, m.reserved_microusd,
       t.limit_microusd, t.spent_microusd, t.reserved_microusd
FROM team_members m
JOIN teams t ON t.id = m.team_id
WHERE m.user_id = ? AND m.team_id = ? AND m.status = 'active'
LIMIT 1`, userID, preferred).Scan(
			&scope.TeamID, &scope.MemberCap, &scope.MemberSpent, &scope.MemberReserved,
			&scope.TeamLimit, &scope.TeamSpent, &scope.TeamReserved,
		)
		if errors.Is(err, sql.ErrNoRows) {
			return teamSpendScope{}, false, fmt.Errorf("%w: not an active member of team %s", ErrTeamNotFound, preferred)
		}
	} else {
		err = q.QueryRowContext(ctx, `
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
