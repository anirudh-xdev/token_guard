package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
)

func TestPortalPageRedirectsToApp(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, portalEnabled: true, portalDevLogin: true})
	client := &http.Client{
		Transport: h.server.Client().Transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(h.url("/portal"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status=%d want 302", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "http://localhost:3000/portal" {
		t.Fatalf("Location=%q", loc)
	}
}

func TestPortalDevLoginCreateKeyAndProxy(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, portalEnabled: true, portalDevLogin: true})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Transport: h.server.Client().Transport}

	status, body := portalDo(t, client, http.MethodPost, h.url("/portal/dev/login"), `{"email":"portal@example.com","name":"Portal User"}`)
	if status != http.StatusOK {
		t.Fatalf("dev login status=%d body=%s", status, body)
	}

	status, body = portalDo(t, client, http.MethodGet, h.url("/portal/api/me"), "")
	if status != http.StatusOK {
		t.Fatalf("me status=%d body=%s", status, body)
	}
	var me map[string]any
	if err := json.Unmarshal([]byte(body), &me); err != nil {
		t.Fatal(err)
	}
	user, _ := me["user"].(map[string]any)
	if user["email"] != "portal@example.com" {
		t.Fatalf("email=%v", user["email"])
	}
	if user["budget_usd"].(float64) != 5 {
		t.Fatalf("default budget=%v want 5", user["budget_usd"])
	}

	status, body = portalDo(t, client, http.MethodPost, h.url("/portal/api/keys"), `{"name":"app"}`)
	if status != http.StatusCreated {
		t.Fatalf("create key status=%d body=%s", status, body)
	}
	var keyOut map[string]any
	if err := json.Unmarshal([]byte(body), &keyOut); err != nil {
		t.Fatal(err)
	}
	apiKey, _ := keyOut["api_key"].(string)
	if !strings.HasPrefix(apiKey, "tg_") {
		t.Fatalf("api_key=%q", apiKey)
	}

	proxyStatus, data, _ := h.doJSON(http.MethodPost, "/v1/chat/completions", map[string]any{
		"model":      "gpt-e2e",
		"messages":   []map[string]string{{"role": "user", "content": "hi"}},
		"max_tokens": 16,
	}, map[string]string{
		"Content-Type":          "application/json",
		"Authorization":         "Bearer sk-test",
		"X-TokenGuard-API-Key":  apiKey,
		"X-TokenGuard-Provider": "openai",
	})
	if proxyStatus != http.StatusOK {
		t.Fatalf("proxy status=%d data=%v", proxyStatus, data)
	}
}

func TestPortalMeUnauthorized(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, portalEnabled: true, portalDevLogin: true})
	status, data, _ := h.doJSON(http.MethodGet, "/portal/api/me", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d data=%v", status, data)
	}
	if data["code"] != "unauthorized" {
		t.Fatalf("code=%v", data["code"])
	}
}

func TestPortalDevLoginDisabled(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, portalEnabled: true, portalDevLogin: false})
	// Harness must supply Clerk placeholders when dev login is off.
	status, _, _ := h.doJSON(http.MethodPost, "/portal/dev/login", map[string]string{
		"email": "x@example.com",
	}, nil)
	if status != http.StatusNotFound {
		t.Fatalf("status=%d want 404", status)
	}
}

func portalDo(t *testing.T, client *http.Client, method, url, body string) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(raw)
}
