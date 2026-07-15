package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultPricingFile = "pricing.json"
	pricingFileEnv     = "TOKENGUARD_PRICING_FILE"
)

type CostEstimate struct {
	Model                       string
	InputTokens                 int64
	MaxOutputTokens             int64
	InputCostMicroUSD           int64
	EstimatedOutputCostMicroUSD int64
	EstimatedTotalCostMicroUSD  int64
	InputCostPer1KMicroUSD      int64
	OutputCostPer1KMicroUSD     int64
}

type PricingEngine struct {
	mu     sync.RWMutex
	prices map[string]Price
}

func PricingFileFromEnv() string {
	if raw := strings.TrimSpace(os.Getenv(pricingFileEnv)); raw != "" {
		return raw
	}
	return defaultPricingFile
}

func LoadPricingFile(ctx context.Context, path string) (*PricingEngine, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if path = strings.TrimSpace(path); path == "" {
		path = defaultPricingFile
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	resolved, err := resolvePricingPath(path)
	if err != nil {
		return nil, fmt.Errorf("read pricing file %q: %w", path, err)
	}

	raw, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read pricing file %q: %w", resolved, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var table map[string]Price
	if err := json.Unmarshal(raw, &table); err != nil {
		return nil, fmt.Errorf("decode pricing file %q: %w", resolved, err)
	}
	return NewPricingEngine(table)
}

// resolvePricingPath tries the given path, then the same relative path next to the executable.
func resolvePricingPath(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return path, nil
	} else if filepath.IsAbs(path) || !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	exe, exeErr := os.Executable()
	if exeErr != nil {
		return "", errFromMissing(path)
	}
	candidate := filepath.Join(filepath.Dir(exe), filepath.Base(path))
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	return "", errFromMissing(path)
}

func errFromMissing(path string) error {
	return fmt.Errorf("pricing file %q not found in working directory or next to the executable", path)
}

func NewPricingEngine(table map[string]Price) (*PricingEngine, error) {
	if len(table) == 0 {
		return nil, errors.New("pricing table is empty")
	}

	prices := make(map[string]Price, len(table))
	for model, price := range table {
		model = strings.TrimSpace(model)
		if model == "" {
			return nil, errors.New("pricing table contains an empty model name")
		}
		if price.InputCostPer1KMicroUSD < 0 {
			return nil, fmt.Errorf("model %q input cost cannot be negative", model)
		}
		if price.OutputCostPer1KMicroUSD < 0 {
			return nil, fmt.Errorf("model %q output cost cannot be negative", model)
		}
		prices[model] = price
	}

	return &PricingEngine{prices: prices}, nil
}

func (e *PricingEngine) PriceForModel(model string) (Price, bool) {
	if e == nil {
		return Price{}, false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	price, ok := e.prices[strings.TrimSpace(model)]
	return price, ok
}

// ModelNames returns configured model ids (sorted for stable API output).
func (e *PricingEngine) ModelNames() []string {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.prices) == 0 {
		return nil
	}
	names := make([]string, 0, len(e.prices))
	for name := range e.prices {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (e *PricingEngine) PriceForProviderModel(provider, model string) (Price, bool) {
	if e == nil {
		return Price{}, false
	}
	provider = strings.ToLower(strings.TrimSpace(provider))
	model = strings.TrimSpace(model)
	if model == "" {
		return Price{}, false
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, key := range modelPriceLookupKeys(provider, model) {
		if price, ok := e.prices[key]; ok {
			return price, true
		}
	}
	return Price{}, false
}

// modelPriceLookupKeys returns candidate catalog keys from most specific to least.
func modelPriceLookupKeys(provider, model string) []string {
	model = strings.TrimSpace(model)
	provider = strings.ToLower(strings.TrimSpace(provider))
	keys := make([]string, 0, 8)
	seen := map[string]struct{}{}
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k == "" {
			return
		}
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}

	if provider != "" {
		add(provider + "/" + model)
		add(provider + ":" + model)
	}
	add(model)

	// OpenRouter-style "vendor/model" also used as bare id when provider=openrouter.
	if i := strings.IndexByte(model, '/'); i > 0 {
		vendor := model[:i]
		leaf := model[i+1:]
		if provider != "" {
			add(provider + "/" + leaf)
		}
		add(leaf)
		if vendor != "" && leaf != "" {
			add(vendor + "/" + leaf)
		}
	}

	// Strip dated snapshots: gpt-4o-mini-2024-07-18 → gpt-4o-mini
	if base := stripModelSnapshotSuffix(model); base != model {
		if provider != "" {
			add(provider + "/" + base)
		}
		add(base)
	}

	return keys
}

func stripModelSnapshotSuffix(model string) string {
	// Common pattern: name-YYYY-MM-DD
	parts := strings.Split(model, "-")
	if len(parts) < 4 {
		return model
	}
	n := len(parts)
	if len(parts[n-3]) == 4 && len(parts[n-2]) == 2 && len(parts[n-1]) == 2 {
		if _, err := strconv.Atoi(parts[n-3]); err == nil {
			return strings.Join(parts[:n-3], "-")
		}
	}
	return model
}

// ReplaceAll atomically replaces the in-memory catalog (used after DB seed/reload).
func (e *PricingEngine) ReplaceAll(table map[string]Price) error {
	if e == nil {
		return errors.New("pricing engine is nil")
	}
	prices := make(map[string]Price, len(table))
	for model, price := range table {
		model = strings.TrimSpace(model)
		if model == "" {
			return errors.New("pricing table contains an empty model name")
		}
		if price.InputCostPer1KMicroUSD < 0 || price.OutputCostPer1KMicroUSD < 0 {
			return fmt.Errorf("model %q has negative cost", model)
		}
		prices[model] = price
	}
	if len(prices) == 0 {
		return errors.New("pricing table is empty")
	}
	e.mu.Lock()
	e.prices = prices
	e.mu.Unlock()
	return nil
}

// Upsert adds or updates one model price in memory.
func (e *PricingEngine) Upsert(model string, price Price) error {
	if e == nil {
		return errors.New("pricing engine is nil")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("model is required")
	}
	if price.InputCostPer1KMicroUSD < 0 || price.OutputCostPer1KMicroUSD < 0 {
		return errors.New("costs cannot be negative")
	}
	e.mu.Lock()
	if e.prices == nil {
		e.prices = make(map[string]Price)
	}
	e.prices[model] = price
	e.mu.Unlock()
	return nil
}

// Delete removes one model price from memory.
func (e *PricingEngine) Delete(model string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	delete(e.prices, strings.TrimSpace(model))
	e.mu.Unlock()
}

func (e *PricingEngine) Snapshot() map[string]Price {
	if e == nil {
		return nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make(map[string]Price, len(e.prices))
	for k, v := range e.prices {
		out[k] = v
	}
	return out
}

func (e *PricingEngine) Estimate(model string, inputTokens, maxOutputTokens int64) (CostEstimate, error) {
	return e.EstimateProvider("", model, inputTokens, maxOutputTokens)
}

func (e *PricingEngine) EstimateProvider(provider, model string, inputTokens, maxOutputTokens int64) (CostEstimate, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return CostEstimate{}, errors.New("model is required")
	}
	if inputTokens < 0 {
		return CostEstimate{}, errors.New("input tokens cannot be negative")
	}
	if maxOutputTokens < 0 {
		return CostEstimate{}, errors.New("max output tokens cannot be negative")
	}

	price, ok := e.PriceForProviderModel(provider, model)
	if !ok {
		if strings.TrimSpace(provider) != "" {
			return CostEstimate{}, fmt.Errorf("pricing not found for provider %q model %q", provider, model)
		}
		return CostEstimate{}, fmt.Errorf("pricing not found for model %q", model)
	}

	inputCost, err := costMicroUSD(inputTokens, price.InputCostPer1KMicroUSD)
	if err != nil {
		return CostEstimate{}, fmt.Errorf("input cost overflow: %w", err)
	}
	outputCost, err := costMicroUSD(maxOutputTokens, price.OutputCostPer1KMicroUSD)
	if err != nil {
		return CostEstimate{}, fmt.Errorf("output cost overflow: %w", err)
	}
	total, err := checkedAdd(inputCost, outputCost)
	if err != nil {
		return CostEstimate{}, fmt.Errorf("total cost overflow: %w", err)
	}

	return CostEstimate{
		Model:                       model,
		InputTokens:                 inputTokens,
		MaxOutputTokens:             maxOutputTokens,
		InputCostMicroUSD:           inputCost,
		EstimatedOutputCostMicroUSD: outputCost,
		EstimatedTotalCostMicroUSD:  total,
		InputCostPer1KMicroUSD:      price.InputCostPer1KMicroUSD,
		OutputCostPer1KMicroUSD:     price.OutputCostPer1KMicroUSD,
	}, nil
}

func (e *PricingEngine) CanAfford(model string, inputTokens, maxOutputTokens, availableMicroUSD int64) (CostEstimate, bool, error) {
	return e.CanAffordProvider("", model, inputTokens, maxOutputTokens, availableMicroUSD)
}

func (e *PricingEngine) CanAffordProvider(provider, model string, inputTokens, maxOutputTokens, availableMicroUSD int64) (CostEstimate, bool, error) {
	if availableMicroUSD < 0 {
		return CostEstimate{}, false, errors.New("available budget cannot be negative")
	}
	estimate, err := e.EstimateProvider(provider, model, inputTokens, maxOutputTokens)
	if err != nil {
		return CostEstimate{}, false, err
	}
	return estimate, estimate.EstimatedTotalCostMicroUSD <= availableMicroUSD, nil
}

func (e *PricingEngine) ActualCostMicroUSD(model string, inputTokens, outputTokens int64) (int64, error) {
	return e.ActualCostMicroUSDProvider("", model, inputTokens, outputTokens)
}

func (e *PricingEngine) ActualCostMicroUSDProvider(provider, model string, inputTokens, outputTokens int64) (int64, error) {
	estimate, err := e.EstimateProvider(provider, model, inputTokens, outputTokens)
	if err != nil {
		return 0, err
	}
	return estimate.EstimatedTotalCostMicroUSD, nil
}

func (e *PricingEngine) ModelCount() int {
	if e == nil {
		return 0
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.prices)
}

func costMicroUSD(tokens, costPer1K int64) (int64, error) {
	if tokens == 0 || costPer1K == 0 {
		return 0, nil
	}
	if tokens > math.MaxInt64/costPer1K {
		return 0, errors.New("multiplication exceeds int64")
	}
	product := tokens * costPer1K
	return ceilDiv(product, 1000), nil
}

func ceilDiv(n, d int64) int64 {
	if n == 0 {
		return 0
	}
	return 1 + (n-1)/d
}

func checkedAdd(a, b int64) (int64, error) {
	if a > math.MaxInt64-b {
		return 0, errors.New("addition exceeds int64")
	}
	return a + b, nil
}
