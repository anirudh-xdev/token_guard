package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultTimeout = 750 * time.Millisecond
	maxBodyBytes   = 1 << 20
)

type Config struct {
	RESTURL   string
	RESTToken string
	Timeout   time.Duration
}

type Client struct {
	restURL string
	token   string
	http    *http.Client
}

type responseEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

func ConfigFromEnv() (Config, error) {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv("TOKENGUARD_REDIS_TIMEOUT_MS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return Config{}, fmt.Errorf("TOKENGUARD_REDIS_TIMEOUT_MS must be a positive integer")
		}
		timeout = time.Duration(parsed) * time.Millisecond
	}

	cfg := Config{
		RESTURL:   strings.TrimRight(strings.TrimSpace(os.Getenv("UPSTASH_REDIS_REST_URL")), "/"),
		RESTToken: strings.TrimSpace(os.Getenv("UPSTASH_REDIS_REST_TOKEN")),
		Timeout:   timeout,
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	var errs []error
	if strings.TrimSpace(c.RESTURL) == "" {
		errs = append(errs, errors.New("UPSTASH_REDIS_REST_URL is required"))
	} else if err := validateRESTURL(c.RESTURL); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(c.RESTToken) == "" {
		errs = append(errs, errors.New("UPSTASH_REDIS_REST_TOKEN is required"))
	}
	if c.Timeout < 0 {
		errs = append(errs, errors.New("Timeout cannot be negative"))
	}
	return errors.Join(errs...)
}

func New(cfg Config) (*Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}

	return &Client{
		restURL: strings.TrimRight(strings.TrimSpace(cfg.RESTURL), "/"),
		token:   strings.TrimSpace(cfg.RESTToken),
		http: &http.Client{
			Timeout: cfg.Timeout,
		},
	}, nil
}

func (c *Client) Command(ctx context.Context, args ...any) (json.RawMessage, error) {
	if c == nil || c.http == nil {
		return nil, errors.New("cache client is nil")
	}
	if len(args) == 0 {
		return nil, errors.New("redis command requires at least one argument")
	}

	body, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal redis command: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.restURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build redis request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute redis command: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("read redis response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("redis returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var envelope responseEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode redis response: %w", err)
	}
	if envelope.Error != "" {
		return nil, errors.New(envelope.Error)
	}
	return envelope.Result, nil
}

func (c *Client) Ping(ctx context.Context) error {
	raw, err := c.Command(ctx, "PING")
	if err != nil {
		return err
	}

	var pong string
	if err := json.Unmarshal(raw, &pong); err != nil {
		return fmt.Errorf("decode ping response: %w", err)
	}
	if pong != "PONG" {
		return fmt.Errorf("unexpected ping response %q", pong)
	}
	return nil
}

func validateRESTURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("parse UPSTASH_REDIS_REST_URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return errors.New("UPSTASH_REDIS_REST_URL must use http or https")
	}
	if parsed.Host == "" {
		return errors.New("UPSTASH_REDIS_REST_URL must include a host")
	}
	return nil
}
