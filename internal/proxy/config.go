package proxy

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListenAddr          = ":8080"
	defaultUpstreamURL         = "https://api.openai.com"
	defaultProviderName        = "openai"
	defaultReadHeaderTimeout   = 2 * time.Second
	defaultShutdownTimeout     = 5 * time.Second
	listenAddrEnv              = "TOKENGUARD_LISTEN_ADDR"
	upstreamURLEnv             = "TOKENGUARD_UPSTREAM_URL"
	defaultProviderEnv         = "TOKENGUARD_DEFAULT_PROVIDER"
	providerRoutesEnv          = "TOKENGUARD_PROVIDER_ROUTES"
	tokenizerModelEnv          = "TOKENGUARD_TOKENIZER_MODEL"
	guardEnabledEnv            = "TOKENGUARD_GUARD_ENABLED"
	managementEnabledEnv       = "TOKENGUARD_MGMT_ENABLED"
	defaultMaxOutputTokensEnv  = "TOKENGUARD_DEFAULT_MAX_OUTPUT_TOKENS"
	maxRequestBytesEnv         = "TOKENGUARD_MAX_REQUEST_BYTES"
	readHeaderTimeoutMillisEnv = "TOKENGUARD_READ_HEADER_TIMEOUT_MS"
	shutdownTimeoutMillisEnv   = "TOKENGUARD_SHUTDOWN_TIMEOUT_MS"
	defaultTokenizerModel      = "gpt-4"
	defaultMaxOutputTokens     = int64(4096)
	defaultMaxRequestBytes     = int64(4 << 20)
)

type Config struct {
	ListenAddr             string
	UpstreamURL            string
	DefaultProvider        string
	ProviderRoutes         map[string]string
	TokenizerModel         string
	GuardEnabled           bool
	ManagementEnabled      bool
	DefaultMaxOutputTokens int64
	MaxRequestBytes        int64
	ReadHeaderTimeout      time.Duration
	ShutdownTimeout        time.Duration
	AdminSecret            string
}

func ConfigFromEnv() (Config, error) {
	defaultMaxOutputTokens, err := int64FromEnv(defaultMaxOutputTokensEnv, defaultMaxOutputTokens)
	if err != nil {
		return Config{}, err
	}

	maxRequestBytes, err := int64FromEnv(maxRequestBytesEnv, defaultMaxRequestBytes)
	if err != nil {
		return Config{}, err
	}

	guardEnabled, err := boolFromEnv(guardEnabledEnv, true)
	if err != nil {
		return Config{}, err
	}

	managementEnabled, err := boolFromEnv(managementEnabledEnv, false)
	if err != nil {
		return Config{}, err
	}

	providerRoutes, err := providerRoutesFromEnv(providerRoutesEnv)
	if err != nil {
		return Config{}, err
	}

	readHeaderTimeout, err := durationFromMillisEnv(readHeaderTimeoutMillisEnv, defaultReadHeaderTimeout)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := durationFromMillisEnv(shutdownTimeoutMillisEnv, defaultShutdownTimeout)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ListenAddr:             strings.TrimSpace(os.Getenv(listenAddrEnv)),
		UpstreamURL:            strings.TrimSpace(os.Getenv(upstreamURLEnv)),
		DefaultProvider:        strings.TrimSpace(os.Getenv(defaultProviderEnv)),
		ProviderRoutes:         providerRoutes,
		TokenizerModel:         strings.TrimSpace(os.Getenv(tokenizerModelEnv)),
		GuardEnabled:           guardEnabled,
		ManagementEnabled:      managementEnabled,
		DefaultMaxOutputTokens: defaultMaxOutputTokens,
		MaxRequestBytes:        maxRequestBytes,
		ReadHeaderTimeout:      readHeaderTimeout,
		ShutdownTimeout:        shutdownTimeout,
		AdminSecret:            strings.TrimSpace(os.Getenv("TOKENGUARD_ADMIN_SECRET")),
	}
	cfg = cfg.withDefaults()

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) withDefaults() Config {
	cfg := c
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	if cfg.UpstreamURL == "" {
		cfg.UpstreamURL = defaultUpstreamURL
	}
	if cfg.DefaultProvider == "" {
		cfg.DefaultProvider = defaultProviderName
	}
	cfg.DefaultProvider = normalizeProviderName(cfg.DefaultProvider)
	if cfg.ProviderRoutes == nil {
		cfg.ProviderRoutes = make(map[string]string, 1)
	}
	if _, ok := cfg.ProviderRoutes[cfg.DefaultProvider]; !ok {
		cfg.ProviderRoutes[cfg.DefaultProvider] = cfg.UpstreamURL
	}
	if _, ok := cfg.ProviderRoutes[defaultProviderName]; !ok {
		cfg.ProviderRoutes[defaultProviderName] = cfg.UpstreamURL
	}
	if cfg.TokenizerModel == "" {
		cfg.TokenizerModel = defaultTokenizerModel
	}
	if cfg.DefaultMaxOutputTokens == 0 {
		cfg.DefaultMaxOutputTokens = defaultMaxOutputTokens
	}
	if cfg.MaxRequestBytes == 0 {
		cfg.MaxRequestBytes = defaultMaxRequestBytes
	}
	return cfg
}

func (c Config) Validate() error {
	var errs []error
	if strings.TrimSpace(c.ListenAddr) == "" {
		errs = append(errs, errors.New("listen address is required"))
	}
	if _, err := parseUpstreamURL(c.UpstreamURL); err != nil {
		errs = append(errs, err)
	}
	if strings.TrimSpace(c.DefaultProvider) == "" {
		errs = append(errs, errors.New("default provider is required"))
	}
	for provider, upstream := range c.ProviderRoutes {
		if strings.TrimSpace(provider) == "" {
			errs = append(errs, errors.New("provider route contains an empty provider name"))
			continue
		}
		if _, err := parseUpstreamURL(upstream); err != nil {
			errs = append(errs, fmt.Errorf("provider route %q: %w", provider, err))
		}
	}
	if strings.TrimSpace(c.TokenizerModel) == "" {
		errs = append(errs, errors.New("tokenizer model is required"))
	}
	if c.DefaultMaxOutputTokens < 0 {
		errs = append(errs, errors.New("default max output tokens cannot be negative"))
	}
	if c.MaxRequestBytes < 0 {
		errs = append(errs, errors.New("max request bytes cannot be negative"))
	}
	if c.ReadHeaderTimeout < 0 {
		errs = append(errs, errors.New("read header timeout cannot be negative"))
	}
	if c.ShutdownTimeout < 0 {
		errs = append(errs, errors.New("shutdown timeout cannot be negative"))
	}
	if c.ManagementEnabled && len(strings.TrimSpace(c.AdminSecret)) < 16 {
		errs = append(errs, errors.New("TOKENGUARD_ADMIN_SECRET must be at least 16 characters when management endpoints are enabled"))
	}
	return errors.Join(errs...)
}

func int64FromEnv(name string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func boolFromEnv(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}

	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return parsed, nil
}

func durationFromMillisEnv(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return time.Duration(parsed) * time.Millisecond, nil
}

func providerRoutesFromEnv(name string) (map[string]string, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil, nil
	}

	routes := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		provider, upstream, ok := strings.Cut(entry, "=")
		if !ok {
			return nil, fmt.Errorf("%s entries must use provider=url", name)
		}
		provider = normalizeProviderName(provider)
		upstream = strings.TrimSpace(upstream)
		if provider == "" || upstream == "" {
			return nil, fmt.Errorf("%s entries must include provider and url", name)
		}
		routes[provider] = upstream
	}
	return routes, nil
}

func parseUpstreamURL(raw string) (*url.URL, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return nil, errors.New("upstream URL is required")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse upstream URL: %w", err)
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return nil, errors.New("upstream URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, errors.New("upstream URL must include a host")
	}
	return parsed, nil
}
