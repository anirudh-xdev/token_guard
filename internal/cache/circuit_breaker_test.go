package cache

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type fakeCommander struct {
	result json.RawMessage
	args   []any
	err    error
}

func (f *fakeCommander) Command(ctx context.Context, args ...any) (json.RawMessage, error) {
	f.args = append([]any(nil), args...)
	return f.result, f.err
}

func TestCircuitBreakerTripsAtThreshold(t *testing.T) {
	redis := &fakeCommander{result: json.RawMessage(`3`)}
	breaker, err := NewCircuitBreaker(redis, CircuitBreakerConfig{
		TTL:       3 * time.Minute,
		Threshold: 3,
		KeyPrefix: "tokenguard:test",
	})
	if err != nil {
		t.Fatalf("NewCircuitBreaker returned error: %v", err)
	}

	result, err := breaker.Check(context.Background(), "session-123", []byte(`{"tool":"lookup"}`))
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if !result.Tripped {
		t.Fatal("Tripped = false, want true")
	}
	if result.Count != 3 {
		t.Fatalf("Count = %d, want 3", result.Count)
	}
	if result.Threshold != 3 {
		t.Fatalf("Threshold = %d, want 3", result.Threshold)
	}
}

func TestCircuitBreakerUsesAtomicLuaCommandAndSafeKey(t *testing.T) {
	redis := &fakeCommander{result: json.RawMessage(`"1"`)}
	breaker, err := NewCircuitBreaker(redis, CircuitBreakerConfig{
		TTL:       90 * time.Second,
		Threshold: 3,
		KeyPrefix: "tokenguard:test:",
	})
	if err != nil {
		t.Fatalf("NewCircuitBreaker returned error: %v", err)
	}

	result, err := breaker.Check(context.Background(), "session:with:colons", []byte("repeat me"))
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if result.Tripped {
		t.Fatal("Tripped = true, want false before threshold")
	}
	if len(redis.args) != 5 {
		t.Fatalf("redis args = %#v, want EVAL script key ttl", redis.args)
	}
	if redis.args[0] != "EVAL" {
		t.Fatalf("command = %v, want EVAL", redis.args[0])
	}
	if redis.args[2] != 1 {
		t.Fatalf("numkeys = %v, want 1", redis.args[2])
	}
	key, ok := redis.args[3].(string)
	if !ok {
		t.Fatalf("key arg type = %T, want string", redis.args[3])
	}
	if strings.Contains(key, "session:with:colons") {
		t.Fatalf("key leaked raw session id: %q", key)
	}
	if !strings.HasPrefix(key, "tokenguard:test:") {
		t.Fatalf("key = %q, want sanitized prefix", key)
	}
	if redis.args[4] != int64(90) {
		t.Fatalf("ttl arg = %#v, want int64(90)", redis.args[4])
	}
}

func TestHashBytesIsStableSHA256Hex(t *testing.T) {
	got := HashBytes([]byte("abc"))
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("HashBytes = %q, want %q", got, want)
	}
}

func TestCircuitBreakerConfigFromEnvDefaults(t *testing.T) {
	t.Setenv(loopTTLSecondsEnv, "")
	t.Setenv(loopThresholdEnv, "")
	t.Setenv(loopKeyPrefixEnv, "")

	cfg, err := CircuitBreakerConfigFromEnv()
	if err != nil {
		t.Fatalf("CircuitBreakerConfigFromEnv returned error: %v", err)
	}
	if cfg.TTL != defaultLoopTTL {
		t.Fatalf("TTL = %v, want %v", cfg.TTL, defaultLoopTTL)
	}
	if cfg.Threshold != defaultLoopThreshold {
		t.Fatalf("Threshold = %d, want %d", cfg.Threshold, defaultLoopThreshold)
	}
	if cfg.KeyPrefix != defaultLoopKeyPrefix {
		t.Fatalf("KeyPrefix = %q, want %q", cfg.KeyPrefix, defaultLoopKeyPrefix)
	}
}
