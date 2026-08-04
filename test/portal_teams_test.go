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
