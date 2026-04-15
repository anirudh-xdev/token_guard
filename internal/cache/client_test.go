package cache

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfigFromEnvRequiresUpstashValues(t *testing.T) {
	t.Setenv("UPSTASH_REDIS_REST_URL", "")
	t.Setenv("UPSTASH_REDIS_REST_TOKEN", "")
	t.Setenv("TOKENGUARD_REDIS_TIMEOUT_MS", "")

	_, err := ConfigFromEnv()
	if err == nil {
		t.Fatal("ConfigFromEnv returned nil error for missing Upstash env")
	}
	if !strings.Contains(err.Error(), "UPSTASH_REDIS_REST_URL") {
		t.Fatalf("error %q does not mention UPSTASH_REDIS_REST_URL", err)
	}
	if !strings.Contains(err.Error(), "UPSTASH_REDIS_REST_TOKEN") {
		t.Fatalf("error %q does not mention UPSTASH_REDIS_REST_TOKEN", err)
	}
}

func TestCommandSendsJSONBodyAndBearerToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var command []any
		if err := json.Unmarshal(body, &command); err != nil {
			t.Fatalf("unmarshal command: %v", err)
		}
		if len(command) != 2 || command[0] != "GET" || command[1] != "loop:key" {
			t.Fatalf("command = %#v, want [GET loop:key]", command)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer server.Close()

	client, err := New(Config{
		RESTURL:   server.URL + "/",
		RESTToken: "test-token",
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	raw, err := client.Command(context.Background(), "GET", "loop:key")
	if err != nil {
		t.Fatalf("Command returned error: %v", err)
	}

	var got string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if got != "ok" {
		t.Fatalf("result = %q, want ok", got)
	}
}

func TestCommandReturnsUpstashError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"ERR bad command"}`))
	}))
	defer server.Close()

	client, err := New(Config{
		RESTURL:   server.URL,
		RESTToken: "test-token",
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = client.Command(context.Background(), "BAD")
	if err == nil {
		t.Fatal("Command returned nil error for Upstash error response")
	}
	if !strings.Contains(err.Error(), "ERR bad command") {
		t.Fatalf("error = %q, want Upstash error", err)
	}
}

func TestPingValidatesPONG(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":"PONG"}`))
	}))
	defer server.Close()

	client, err := New(Config{
		RESTURL:   server.URL,
		RESTToken: "test-token",
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping returned error: %v", err)
	}
}
