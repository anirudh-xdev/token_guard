package models

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEstimateUsesCeilingMicroUSDCosts(t *testing.T) {
	engine, err := NewPricingEngine(map[string]Price{
		"test-model": {
			InputCostPer1KMicroUSD:  150,
			OutputCostPer1KMicroUSD: 600,
		},
	})
	if err != nil {
		t.Fatalf("NewPricingEngine returned error: %v", err)
	}

	estimate, err := engine.Estimate("test-model", 1001, 501)
	if err != nil {
		t.Fatalf("Estimate returned error: %v", err)
	}

	if estimate.InputCostMicroUSD != 151 {
		t.Fatalf("InputCostMicroUSD = %d, want 151", estimate.InputCostMicroUSD)
	}
	if estimate.EstimatedOutputCostMicroUSD != 301 {
		t.Fatalf("EstimatedOutputCostMicroUSD = %d, want 301", estimate.EstimatedOutputCostMicroUSD)
	}
	if estimate.EstimatedTotalCostMicroUSD != 452 {
		t.Fatalf("EstimatedTotalCostMicroUSD = %d, want 452", estimate.EstimatedTotalCostMicroUSD)
	}
}

func TestCanAffordComparesAgainstAvailableBudget(t *testing.T) {
	engine, err := NewPricingEngine(map[string]Price{
		"test-model": {
			InputCostPer1KMicroUSD:  1000,
			OutputCostPer1KMicroUSD: 1000,
		},
	})
	if err != nil {
		t.Fatalf("NewPricingEngine returned error: %v", err)
	}

	_, ok, err := engine.CanAfford("test-model", 500, 500, 999)
	if err != nil {
		t.Fatalf("CanAfford returned error: %v", err)
	}
	if ok {
		t.Fatal("CanAfford returned true for insufficient budget")
	}

	_, ok, err = engine.CanAfford("test-model", 500, 500, 1000)
	if err != nil {
		t.Fatalf("CanAfford returned error: %v", err)
	}
	if !ok {
		t.Fatal("CanAfford returned false for exact budget")
	}
}

func TestProviderPricingPrefersProviderScopedModel(t *testing.T) {
	engine, err := NewPricingEngine(map[string]Price{
		"shared-model": {
			InputCostPer1KMicroUSD:  1000,
			OutputCostPer1KMicroUSD: 1000,
		},
		"anthropic/shared-model": {
			InputCostPer1KMicroUSD:  2000,
			OutputCostPer1KMicroUSD: 4000,
		},
	})
	if err != nil {
		t.Fatalf("NewPricingEngine returned error: %v", err)
	}

	estimate, err := engine.EstimateProvider("anthropic", "shared-model", 1000, 1000)
	if err != nil {
		t.Fatalf("EstimateProvider returned error: %v", err)
	}
	if estimate.EstimatedTotalCostMicroUSD != 6000 {
		t.Fatalf("EstimatedTotalCostMicroUSD = %d, want provider scoped price", estimate.EstimatedTotalCostMicroUSD)
	}
}

func TestActualCostMicroUSDUsesObservedOutputTokens(t *testing.T) {
	engine, err := NewPricingEngine(map[string]Price{
		"test-model": {
			InputCostPer1KMicroUSD:  1000,
			OutputCostPer1KMicroUSD: 2000,
		},
	})
	if err != nil {
		t.Fatalf("NewPricingEngine returned error: %v", err)
	}

	got, err := engine.ActualCostMicroUSD("test-model", 1000, 250)
	if err != nil {
		t.Fatalf("ActualCostMicroUSD returned error: %v", err)
	}
	if got != 1500 {
		t.Fatalf("ActualCostMicroUSD = %d, want 1500", got)
	}
}

func TestLoadPricingFileValidatesModelTable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	if err := os.WriteFile(path, []byte(`{"gpt-test":{"input_cost_per_1k":1,"output_cost_per_1k":2}}`), 0600); err != nil {
		t.Fatalf("write pricing file: %v", err)
	}

	engine, err := LoadPricingFile(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadPricingFile returned error: %v", err)
	}
	if engine.ModelCount() != 1 {
		t.Fatalf("ModelCount = %d, want 1", engine.ModelCount())
	}
}

func TestLoadPricingFileAcceptsUSDPerMillion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	raw := `{"gpt-4o-mini":{"input_usd_per_million":0.15,"output_usd_per_million":0.6}}`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatalf("write pricing file: %v", err)
	}
	engine, err := LoadPricingFile(context.Background(), path)
	if err != nil {
		t.Fatalf("LoadPricingFile: %v", err)
	}
	price, ok := engine.PriceForModel("gpt-4o-mini")
	if !ok {
		t.Fatal("model missing")
	}
	if price.InputCostPer1KMicroUSD != 150 {
		t.Fatalf("input = %d, want 150 micro/1K ($0.15/1M)", price.InputCostPer1KMicroUSD)
	}
	if price.OutputCostPer1KMicroUSD != 600 {
		t.Fatalf("output = %d, want 600 micro/1K ($0.60/1M)", price.OutputCostPer1KMicroUSD)
	}
}

func TestPriceForProviderModelResolvesAliases(t *testing.T) {
	engine, err := NewPricingEngine(map[string]Price{
		"gpt-4o-mini": {
			InputCostPer1KMicroUSD:  150,
			OutputCostPer1KMicroUSD: 600,
		},
	})
	if err != nil {
		t.Fatalf("NewPricingEngine: %v", err)
	}
	_, ok := engine.PriceForProviderModel("openrouter", "openai/gpt-4o-mini")
	if !ok {
		t.Fatal("expected leaf alias openai/gpt-4o-mini → gpt-4o-mini")
	}
	_, ok = engine.PriceForProviderModel("openai", "gpt-4o-mini-2024-07-18")
	if !ok {
		t.Fatal("expected snapshot suffix strip")
	}
}

func TestOpenRouterUSDPerTokenConversion(t *testing.T) {
	// $0.15 / 1M = 0.00000015 USD/token → 150 micro-USD per 1K
	got, err := OpenRouterUSDPerTokenToMicroPer1K("0.00000015")
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if got != 150 {
		t.Fatalf("got %d, want 150", got)
	}
}

func TestNewPricingEngineRejectsNegativeCosts(t *testing.T) {
	_, err := NewPricingEngine(map[string]Price{
		"bad-model": {
			InputCostPer1KMicroUSD: -1,
		},
	})
	if err == nil {
		t.Fatal("NewPricingEngine returned nil error for negative cost")
	}
	if !strings.Contains(err.Error(), "bad-model") {
		t.Fatalf("error = %q, want model name", err)
	}
}
