package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLoopTTL       = 3 * time.Minute
	defaultLoopThreshold = 3
	defaultLoopKeyPrefix = "tokenguard:loop"

	loopTTLSecondsEnv = "TOKENGUARD_LOOP_TTL_SECONDS"
	loopThresholdEnv  = "TOKENGUARD_LOOP_THRESHOLD"
	loopKeyPrefixEnv  = "TOKENGUARD_LOOP_KEY_PREFIX"
)

const loopCounterScript = `
local count = redis.call("INCR", KEYS[1])
if count == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return count
`

type Commander interface {
	Command(ctx context.Context, args ...any) (json.RawMessage, error)
}

type CircuitBreakerConfig struct {
	TTL       time.Duration
	Threshold int64
	KeyPrefix string
}

type CircuitBreaker struct {
	redis     Commander
	ttl       time.Duration
	threshold int64
	keyPrefix string
}

type CircuitBreakerResult struct {
	SessionHash string
	PayloadHash string
	Key         string
	Count       int64
	Threshold   int64
	TTL         time.Duration
	Tripped     bool
}

func CircuitBreakerConfigFromEnv() (CircuitBreakerConfig, error) {
	ttl := defaultLoopTTL
	if raw := strings.TrimSpace(os.Getenv(loopTTLSecondsEnv)); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return CircuitBreakerConfig{}, fmt.Errorf("%s must be a positive integer", loopTTLSecondsEnv)
		}
		ttl = time.Duration(parsed) * time.Second
	}

	threshold := int64(defaultLoopThreshold)
	if raw := strings.TrimSpace(os.Getenv(loopThresholdEnv)); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return CircuitBreakerConfig{}, fmt.Errorf("%s must be a positive integer", loopThresholdEnv)
		}
		threshold = int64(parsed)
	}

	keyPrefix := strings.TrimSpace(os.Getenv(loopKeyPrefixEnv))
	if keyPrefix == "" {
		keyPrefix = defaultLoopKeyPrefix
	}

	cfg := CircuitBreakerConfig{
		TTL:       ttl,
		Threshold: threshold,
		KeyPrefix: keyPrefix,
	}
	if err := cfg.Validate(); err != nil {
		return CircuitBreakerConfig{}, err
	}
	return cfg, nil
}

func (c CircuitBreakerConfig) withDefaults() CircuitBreakerConfig {
	cfg := c
	if cfg.TTL == 0 {
		cfg.TTL = defaultLoopTTL
	}
	if cfg.Threshold == 0 {
		cfg.Threshold = defaultLoopThreshold
	}
	if strings.TrimSpace(cfg.KeyPrefix) == "" {
		cfg.KeyPrefix = defaultLoopKeyPrefix
	}
	cfg.KeyPrefix = strings.TrimRight(strings.TrimSpace(cfg.KeyPrefix), ":")
	return cfg
}

func (c CircuitBreakerConfig) Validate() error {
	var errs []error
	if c.TTL < 0 {
		errs = append(errs, errors.New("loop TTL cannot be negative"))
	}
	if c.Threshold < 0 {
		errs = append(errs, errors.New("loop threshold cannot be negative"))
	}
	return errors.Join(errs...)
}

func NewCircuitBreaker(redis Commander, cfg CircuitBreakerConfig) (*CircuitBreaker, error) {
	if redis == nil {
		return nil, errors.New("redis commander is required")
	}
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &CircuitBreaker{
		redis:     redis,
		ttl:       cfg.TTL,
		threshold: cfg.Threshold,
		keyPrefix: cfg.KeyPrefix,
	}, nil
}

func (b *CircuitBreaker) Check(ctx context.Context, sessionID string, payload []byte) (CircuitBreakerResult, error) {
	if b == nil || b.redis == nil {
		return CircuitBreakerResult{}, errors.New("circuit breaker is nil")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return CircuitBreakerResult{}, errors.New("session_id is required")
	}
	if len(payload) == 0 {
		return CircuitBreakerResult{}, errors.New("payload is required")
	}

	sessionHash := HashText(sessionID)
	payloadHash := HashBytes(payload)
	key := b.key(sessionHash, payloadHash)
	ttlSeconds := int64(b.ttl / time.Second)
	if ttlSeconds <= 0 {
		ttlSeconds = int64(defaultLoopTTL / time.Second)
	}

	raw, err := b.redis.Command(ctx, "EVAL", loopCounterScript, 1, key, ttlSeconds)
	if err != nil {
		return CircuitBreakerResult{}, fmt.Errorf("increment loop counter: %w", err)
	}

	count, err := decodeRedisInt(raw)
	if err != nil {
		return CircuitBreakerResult{}, err
	}

	return CircuitBreakerResult{
		SessionHash: sessionHash,
		PayloadHash: payloadHash,
		Key:         key,
		Count:       count,
		Threshold:   b.threshold,
		TTL:         b.ttl,
		Tripped:     count >= b.threshold,
	}, nil
}

func (b *CircuitBreaker) key(sessionHash, payloadHash string) string {
	return b.keyPrefix + ":" + sessionHash + ":" + payloadHash
}

func HashText(text string) string {
	return HashBytes([]byte(text))
}

func HashBytes(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func decodeRedisInt(raw json.RawMessage) (int64, error) {
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return n, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		parsed, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse redis integer %q: %w", s, err)
		}
		return parsed, nil
	}
	return 0, fmt.Errorf("decode redis integer from %s", string(raw))
}
