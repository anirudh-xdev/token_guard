package models

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmptyPricingEngineAllowed(t *testing.T) {
	engine, err := NewPricingEngine(nil)
	if err != nil {
		t.Fatalf("NewPricingEngine(nil): %v", err)
	}
	if engine.ModelCount() != 0 {
		t.Fatalf("ModelCount = %d, want 0", engine.ModelCount())
	}
	empty := EmptyPricingEngine()
	if empty.ModelCount() != 0 {
		t.Fatalf("EmptyPricingEngine count = %d", empty.ModelCount())
	}
}

func TestLoadPricingFileOrEmptyMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	engine, err := LoadPricingFileOrEmpty(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadPricingFileOrEmpty: %v", err)
	}
	if engine.ModelCount() != 0 {
		t.Fatalf("expected empty catalog, got %d", engine.ModelCount())
	}
}

func TestLoadPricingFileOrEmptyLoadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	if err := os.WriteFile(path, []byte(`{"m1":{"input_usd_per_million":1,"output_usd_per_million":2}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	engine, err := LoadPricingFileOrEmpty(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadPricingFileOrEmpty: %v", err)
	}
	if engine.ModelCount() != 1 {
		t.Fatalf("ModelCount = %d, want 1", engine.ModelCount())
	}
}

func TestPricingSyncOpenRouterEnabledDefaultAndFalse(t *testing.T) {
	t.Setenv("TOKENGUARD_PRICING_SYNC_OPENROUTER", "")
	if !PricingSyncOpenRouterEnabled() {
		t.Fatal("unset env should default to true")
	}
	t.Setenv("TOKENGUARD_PRICING_SYNC_OPENROUTER", "false")
	if PricingSyncOpenRouterEnabled() {
		t.Fatal("false should disable sync")
	}
	t.Setenv("TOKENGUARD_PRICING_SYNC_OPENROUTER", "true")
	if !PricingSyncOpenRouterEnabled() {
		t.Fatal("true should enable sync")
	}
}

func TestPricingSyncInterval(t *testing.T) {
	t.Setenv("TOKENGUARD_PRICING_SYNC_INTERVAL", "")
	if got := PricingSyncInterval(); got != 6*time.Hour {
		t.Fatalf("default interval = %s, want 6h", got)
	}
	t.Setenv("TOKENGUARD_PRICING_SYNC_INTERVAL", "0")
	if got := PricingSyncInterval(); got != 0 {
		t.Fatalf("0 interval = %s, want 0", got)
	}
	t.Setenv("TOKENGUARD_PRICING_SYNC_INTERVAL", "30m")
	if got := PricingSyncInterval(); got != 30*time.Minute {
		t.Fatalf("interval = %s, want 30m", got)
	}
}
