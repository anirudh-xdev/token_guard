package e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPublicHealthz(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})
	status, body, _ := h.do(http.MethodGet, "/healthz", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Fatalf("body = %s", body)
	}
}

func TestPublicDocs(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})
	status, body, hdr := h.do(http.MethodGet, "/docs", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	if !strings.Contains(hdr.Get("Content-Type"), "text/html") {
		t.Fatalf("Content-Type = %q", hdr.Get("Content-Type"))
	}
	if !strings.Contains(string(body), "TokenGuard") {
		t.Fatal("docs HTML missing TokenGuard")
	}
}

func TestDiscoveryJSON(t *testing.T) {
	h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})
	status, data, _ := h.doJSON(http.MethodGet, "/v1/tokenguard.json", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d data=%v", status, data)
	}
	if data["service"] != "tokenguard" {
		t.Fatalf("service = %v", data["service"])
	}
	if data["guard_enabled"] != true {
		t.Fatalf("guard_enabled = %v", data["guard_enabled"])
	}
	if data["management_enabled"] != true {
		t.Fatalf("management_enabled = %v", data["management_enabled"])
	}
	if _, ok := data["providers"]; !ok {
		t.Fatal("missing providers")
	}
	if _, ok := data["provider_bases"]; !ok {
		t.Fatal("missing provider_bases")
	}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), adminSecret) {
		t.Fatal("discovery leaked admin secret")
	}
	if strings.Contains(string(raw), h.apiKey) {
		t.Fatal("discovery leaked api key")
	}
}

func TestDashboardGating(t *testing.T) {
	t.Run("mgmt_on", func(t *testing.T) {
		h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: true})
		status, body, _ := h.do(http.MethodGet, "/dashboard", nil, nil)
		if status != http.StatusOK {
			t.Fatalf("status = %d", status)
		}
		if !strings.Contains(string(body), "Unlock console") {
			t.Fatal("dashboard missing unlock flow")
		}
	})
	t.Run("mgmt_off", func(t *testing.T) {
		h := newHarness(t, harnessOpts{guardEnabled: true, mgmtEnabled: false})
		_, body, _ := h.do(http.MethodGet, "/dashboard", nil, nil)
		if strings.Contains(string(body), "Unlock console") {
			t.Fatal("dashboard should not be registered when mgmt disabled")
		}
	})
}
