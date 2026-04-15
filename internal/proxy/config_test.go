package proxy

import (
	"testing"
	"time"
)

func TestConfigFromEnvUsesDefaults(t *testing.T) {
	t.Setenv(listenAddrEnv, "")
	t.Setenv(upstreamURLEnv, "")
	t.Setenv(tokenizerModelEnv, "")
	t.Setenv(guardEnabledEnv, "")
	t.Setenv(defaultMaxOutputTokensEnv, "")
	t.Setenv(maxRequestBytesEnv, "")
	t.Setenv(readHeaderTimeoutMillisEnv, "")
	t.Setenv(shutdownTimeoutMillisEnv, "")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv returned error: %v", err)
	}
	if cfg.ListenAddr != defaultListenAddr {
		t.Fatalf("ListenAddr = %q, want %q", cfg.ListenAddr, defaultListenAddr)
	}
	if cfg.UpstreamURL != defaultUpstreamURL {
		t.Fatalf("UpstreamURL = %q, want %q", cfg.UpstreamURL, defaultUpstreamURL)
	}
	if cfg.TokenizerModel != defaultTokenizerModel {
		t.Fatalf("TokenizerModel = %q, want %q", cfg.TokenizerModel, defaultTokenizerModel)
	}
	if !cfg.GuardEnabled {
		t.Fatal("GuardEnabled = false, want true by default")
	}
	if cfg.DefaultMaxOutputTokens != defaultMaxOutputTokens {
		t.Fatalf("DefaultMaxOutputTokens = %d, want %d", cfg.DefaultMaxOutputTokens, defaultMaxOutputTokens)
	}
	if cfg.MaxRequestBytes != defaultMaxRequestBytes {
		t.Fatalf("MaxRequestBytes = %d, want %d", cfg.MaxRequestBytes, defaultMaxRequestBytes)
	}
	if cfg.ReadHeaderTimeout != defaultReadHeaderTimeout {
		t.Fatalf("ReadHeaderTimeout = %v, want %v", cfg.ReadHeaderTimeout, defaultReadHeaderTimeout)
	}
}

func TestConfigFromEnvParsesTimeouts(t *testing.T) {
	t.Setenv(listenAddrEnv, ":9090")
	t.Setenv(upstreamURLEnv, "https://example.test")
	t.Setenv(tokenizerModelEnv, "gpt-4o")
	t.Setenv(guardEnabledEnv, "false")
	t.Setenv(defaultMaxOutputTokensEnv, "1024")
	t.Setenv(maxRequestBytesEnv, "2048")
	t.Setenv(readHeaderTimeoutMillisEnv, "1500")
	t.Setenv(shutdownTimeoutMillisEnv, "3000")

	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv returned error: %v", err)
	}
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("ListenAddr = %q, want :9090", cfg.ListenAddr)
	}
	if cfg.UpstreamURL != "https://example.test" {
		t.Fatalf("UpstreamURL = %q, want https://example.test", cfg.UpstreamURL)
	}
	if cfg.TokenizerModel != "gpt-4o" {
		t.Fatalf("TokenizerModel = %q, want gpt-4o", cfg.TokenizerModel)
	}
	if cfg.GuardEnabled {
		t.Fatal("GuardEnabled = true, want false")
	}
	if cfg.DefaultMaxOutputTokens != 1024 {
		t.Fatalf("DefaultMaxOutputTokens = %d, want 1024", cfg.DefaultMaxOutputTokens)
	}
	if cfg.MaxRequestBytes != 2048 {
		t.Fatalf("MaxRequestBytes = %d, want 2048", cfg.MaxRequestBytes)
	}
	if cfg.ReadHeaderTimeout != 1500*time.Millisecond {
		t.Fatalf("ReadHeaderTimeout = %v, want 1500ms", cfg.ReadHeaderTimeout)
	}
	if cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("ShutdownTimeout = %v, want 3s", cfg.ShutdownTimeout)
	}
}

func TestParseUpstreamURLRejectsMissingHost(t *testing.T) {
	if _, err := parseUpstreamURL("https://"); err == nil {
		t.Fatal("parseUpstreamURL returned nil error for missing host")
	}
}
