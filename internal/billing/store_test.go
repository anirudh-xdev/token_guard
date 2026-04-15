package billing

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
	"testing"
)

func TestBuildDatabaseURLSetsEscapedAuthTokenOnce(t *testing.T) {
	got, err := BuildDatabaseURL("libsql://example.turso.io?authToken=old&authToken=older&foo=bar", "tok+/= value")
	if err != nil {
		t.Fatalf("BuildDatabaseURL returned error: %v", err)
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("parse result: %v", err)
	}

	values := parsed.Query()
	if values.Get("authToken") != "tok+/= value" {
		t.Fatalf("authToken = %q, want %q", values.Get("authToken"), "tok+/= value")
	}
	if got := values["authToken"]; len(got) != 1 {
		t.Fatalf("authToken values = %v, want exactly one", got)
	}
	if values.Get("foo") != "bar" {
		t.Fatalf("foo query param was not preserved")
	}
	if strings.Contains(parsed.RawQuery, "tok+/= value") {
		t.Fatalf("auth token was not URL-escaped in raw query: %q", parsed.RawQuery)
	}
}

func TestConfigFromEnvRequiresTursoValues(t *testing.T) {
	t.Setenv("TURSO_DATABASE_URL", "")
	t.Setenv("TURSO_AUTH_TOKEN", "")
	t.Setenv("TOKENGUARD_DB_MAX_OPEN_CONNS", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("ConfigFromEnv returned nil error for missing Turso env")
	}
	if !strings.Contains(err.Error(), "TURSO_DATABASE_URL") {
		t.Fatalf("error %q does not mention TURSO_DATABASE_URL", err)
	}
	if !strings.Contains(err.Error(), "TURSO_AUTH_TOKEN") {
		t.Fatalf("error %q does not mention TURSO_AUTH_TOKEN", err)
	}
}

func TestHashAPIKeyUsesTrimmedSHA256Hex(t *testing.T) {
	sum := sha256.Sum256([]byte("tg_test"))
	want := hex.EncodeToString(sum[:])

	got := HashAPIKey("  tg_test  ")
	if got != want {
		t.Fatalf("HashAPIKey = %q, want %q", got, want)
	}
}
