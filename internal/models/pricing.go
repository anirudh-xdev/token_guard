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
	"strings"
)

const (
	defaultPricingFile = "pricing.json"
	pricingFileEnv     = "TOKENGUARD_PRICING_FILE"
)

type Price struct {
	InputCostPer1KMicroUSD  int64 `json:"input_cost_per_1k"`
	OutputCostPer1KMicroUSD int64 `json:"output_cost_per_1k"`
}

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
	price, ok := e.prices[strings.TrimSpace(model)]
	return price, ok
}

// ModelNames returns configured model ids (sorted for stable API output).
func (e *PricingEngine) ModelNames() []string {
	if e == nil || len(e.prices) == 0 {
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
	if provider != "" && model != "" {
		for _, key := range []string{provider + "/" + model, provider + ":" + model} {
			if price, ok := e.prices[key]; ok {
				return price, true
			}
		}
	}
	return e.PriceForModel(model)
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
