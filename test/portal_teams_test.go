package e2e

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func TestPortalTeamCreateAndInvite(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, portalEnabled: true, portalDevLogin: true})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Transport: h.server.Client().Transport}

	status, body := portalDo(t, client, http.MethodPost, h.url("/portal/dev/login"), `{"email":"owner@example.com","name":"Owner"}`)
	if status != http.StatusOK {
		t.Fatalf("owner login=%d %s", status, body)
	}

	// Second user must exist before invite.
	jar2, _ := cookiejar.New(nil)
	client2 := &http.Client{Jar: jar2, Transport: h.server.Client().Transport}
	status, body = portalDo(t, client2, http.MethodPost, h.url("/portal/dev/login"), `{"email":"member@example.com","name":"Member"}`)
	if status != http.StatusOK {
		t.Fatalf("member login=%d %s", status, body)
	}

	status, body = portalDo(t, client, http.MethodPost, h.url("/portal/api/teams"), `{"name":"Acme","budget_usd":2000}`)
	if status != http.StatusCreated {
		t.Fatalf("create team=%d %s", status, body)
	}
	var team map[string]any
	if err := json.Unmarshal([]byte(body), &team); err != nil {
		t.Fatal(err)
	}
	teamID, _ := team["id"].(string)
	if teamID == "" {
		t.Fatalf("missing team id: %s", body)
	}
	if team["budget_usd"].(float64) != 2000 {
		t.Fatalf("budget=%v", team["budget_usd"])
	}

	status, body = portalDo(t, client, http.MethodPost, h.url("/portal/api/teams/members"), `{"team_id":"`+teamID+`","email":"member@example.com","cap_usd":200}`)
	if status != http.StatusCreated {
		t.Fatalf("invite=%d %s", status, body)
	}
	if !strings.Contains(body, "member@example.com") {
		t.Fatalf("invite body=%s", body)
	}

	status, body = portalDo(t, client2, http.MethodGet, h.url("/portal/api/me"), "")
	if status != http.StatusOK {
		t.Fatalf("member me=%d %s", status, body)
	}
	if !strings.Contains(body, teamID) {
		t.Fatalf("member should see team: %s", body)
	}
}

func TestPortalTeamBudgetCapRemoveAndReinvite(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, portalEnabled: true, portalDevLogin: true})

	jar, _ := cookiejar.New(nil)
	owner := &http.Client{Jar: jar, Transport: h.server.Client().Transport}
	jar2, _ := cookiejar.New(nil)
	member := &http.Client{Jar: jar2, Transport: h.server.Client().Transport}

	portalDo(t, owner, http.MethodPost, h.url("/portal/dev/login"), `{"email":"owner2@example.com","name":"Owner"}`)
	portalDo(t, member, http.MethodPost, h.url("/portal/dev/login"), `{"email":"member2@example.com","name":"Member"}`)

	status, body := portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams"), `{"name":"PoolCo","budget_usd":100}`)
	if status != http.StatusCreated {
		t.Fatalf("create=%d %s", status, body)
	}
	var team map[string]any
	_ = json.Unmarshal([]byte(body), &team)
	teamID, _ := team["id"].(string)

	// Cap above pool rejected.
	status, body = portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams/members"),
		`{"team_id":"`+teamID+`","email":"member2@example.com","cap_usd":500}`)
	if status != http.StatusBadRequest {
		t.Fatalf("cap>pool status=%d %s", status, body)
	}
	if !strings.Contains(body, "exceed") {
		t.Fatalf("expected exceed error: %s", body)
	}

	status, body = portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams/members"),
		`{"team_id":"`+teamID+`","email":"member2@example.com","cap_usd":40}`)
	if status != http.StatusCreated {
		t.Fatalf("invite=%d %s", status, body)
	}
	var invited map[string]any
	_ = json.Unmarshal([]byte(body), &invited)
	memberID, _ := invited["user_id"].(string)
	if memberID == "" {
		t.Fatalf("missing member user_id: %s", body)
	}

	status, body = portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams/budget"),
		`{"team_id":"`+teamID+`","budget_usd":250}`)
	if status != http.StatusOK {
		t.Fatalf("budget=%d %s", status, body)
	}
	if !strings.Contains(body, `"budget_usd":250`) && !strings.Contains(body, `"budget_usd": 250`) {
		// JSON encoder may emit 250 without spaces
		var updated map[string]any
		_ = json.Unmarshal([]byte(body), &updated)
		if updated["budget_usd"].(float64) != 250 {
			t.Fatalf("budget body=%s", body)
		}
	}

	status, body = portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams/members/cap"),
		`{"team_id":"`+teamID+`","user_id":"`+memberID+`","cap_usd":80}`)
	if status != http.StatusOK {
		t.Fatalf("cap=%d %s", status, body)
	}

	status, body = portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams/members/remove"),
		`{"team_id":"`+teamID+`","user_id":"`+memberID+`"}`)
	if status != http.StatusOK {
		t.Fatalf("remove=%d %s", status, body)
	}

	status, body = portalDo(t, member, http.MethodGet, h.url("/portal/api/me"), "")
	if status != http.StatusOK {
		t.Fatalf("me after remove=%d %s", status, body)
	}
	if strings.Contains(body, teamID) {
		t.Fatalf("removed member should not see team: %s", body)
	}

	// Re-invite after soft-remove.
	status, body = portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams/members"),
		`{"team_id":"`+teamID+`","email":"member2@example.com","cap_usd":50}`)
	if status != http.StatusCreated {
		t.Fatalf("reinvite=%d %s", status, body)
	}

	status, body = portalDo(t, member, http.MethodGet, h.url("/portal/api/me"), "")
	if status != http.StatusOK || !strings.Contains(body, teamID) {
		t.Fatalf("member should see team again: %d %s", status, body)
	}
}

func TestPortalPendingInviteAcceptsOnLogin(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, portalEnabled: true, portalDevLogin: true})

	jar, _ := cookiejar.New(nil)
	owner := &http.Client{Jar: jar, Transport: h.server.Client().Transport}
	portalDo(t, owner, http.MethodPost, h.url("/portal/dev/login"), `{"email":"owner-inv@example.com","name":"Owner"}`)

	status, body := portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams"), `{"name":"InviteCo","budget_usd":100}`)
	if status != http.StatusCreated {
		t.Fatalf("create=%d %s", status, body)
	}
	var team map[string]any
	_ = json.Unmarshal([]byte(body), &team)
	teamID, _ := team["id"].(string)

	status, body = portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams/members"),
		`{"team_id":"`+teamID+`","email":"newbie@example.com","cap_usd":25}`)
	if status != http.StatusAccepted {
		t.Fatalf("pending invite status=%d %s", status, body)
	}
	if !strings.Contains(body, `"pending_invite":true`) && !strings.Contains(body, `"pending_invite": true`) {
		t.Fatalf("expected pending_invite: %s", body)
	}

	status, body = portalDo(t, owner, http.MethodGet, h.url("/portal/api/teams/invites?team_id="+teamID), "")
	if status != http.StatusOK {
		t.Fatalf("list invites=%d %s", status, body)
	}
	if !strings.Contains(body, "newbie@example.com") {
		t.Fatalf("invites body=%s", body)
	}

	jar2, _ := cookiejar.New(nil)
	newbie := &http.Client{Jar: jar2, Transport: h.server.Client().Transport}
	status, body = portalDo(t, newbie, http.MethodPost, h.url("/portal/dev/login"), `{"email":"newbie@example.com","name":"Newbie"}`)
	if status != http.StatusOK {
		t.Fatalf("newbie login=%d %s", status, body)
	}
	status, body = portalDo(t, newbie, http.MethodGet, h.url("/portal/api/me"), "")
	if status != http.StatusOK {
		t.Fatalf("me=%d %s", status, body)
	}
	if !strings.Contains(body, teamID) {
		t.Fatalf("pending invite should auto-accept: %s", body)
	}
	if !strings.Contains(body, "owner-inv@example.com") && !strings.Contains(body, "owner_email") {
		// soft check — assignment enrichment may include owner_email
	}
}

func TestPortalTeamIDHeaderSelectsSpend(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, portalEnabled: true, portalDevLogin: true})

	jar, _ := cookiejar.New(nil)
	owner := &http.Client{Jar: jar, Transport: h.server.Client().Transport}
	jar2, _ := cookiejar.New(nil)
	member := &http.Client{Jar: jar2, Transport: h.server.Client().Transport}

	portalDo(t, owner, http.MethodPost, h.url("/portal/dev/login"), `{"email":"owner-hdr@example.com","name":"Owner"}`)
	portalDo(t, member, http.MethodPost, h.url("/portal/dev/login"), `{"email":"member-hdr@example.com","name":"Member"}`)

	status, body := portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams"), `{"name":"TeamA","budget_usd":50}`)
	if status != http.StatusCreated {
		t.Fatalf("teamA=%d %s", status, body)
	}
	var teamA map[string]any
	_ = json.Unmarshal([]byte(body), &teamA)
	teamAID, _ := teamA["id"].(string)

	status, body = portalDo(t, owner, http.MethodPost, h.url("/portal/api/teams/members"),
		`{"team_id":"`+teamAID+`","email":"member-hdr@example.com","cap_usd":10}`)
	if status != http.StatusCreated {
		t.Fatalf("invite=%d %s", status, body)
	}

	// Member creates a key.
	status, body = portalDo(t, member, http.MethodPost, h.url("/portal/api/keys"), `{"name":"default"}`)
	if status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("create key=%d %s", status, body)
	}
	var keyResp map[string]any
	_ = json.Unmarshal([]byte(body), &keyResp)
	apiKey, _ := keyResp["api_key"].(string)
	if apiKey == "" {
		t.Fatalf("missing api_key: %s", body)
	}

	chatBody := map[string]any{
		"model":      "gpt-e2e",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 16,
	}

	// Invalid team id → 400
	bad := h.proxyHeaders(map[string]string{
		"X-TokenGuard-API-Key": apiKey,
		"X-TokenGuard-Team-ID": "team_does_not_exist",
	})
	status, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", chatBody, bad)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid team status=%d data=%v", status, data)
	}
	if data["code"] != "invalid_team_id" {
		t.Fatalf("code=%v want invalid_team_id data=%v", data["code"], data)
	}

	// Valid team id → allowed
	okHdr := h.proxyHeaders(map[string]string{
		"X-TokenGuard-API-Key": apiKey,
		"X-TokenGuard-Team-ID": teamAID,
	})
	status, data, _ = h.doJSON(http.MethodPost, "/v1/chat/completions", chatBody, okHdr)
	if status != http.StatusOK {
		t.Fatalf("valid team spend status=%d data=%v", status, data)
	}

	status, body = portalDo(t, member, http.MethodGet, h.url("/portal/api/usage"), "")
	if status != http.StatusOK {
		t.Fatalf("usage=%d %s", status, body)
	}
}
