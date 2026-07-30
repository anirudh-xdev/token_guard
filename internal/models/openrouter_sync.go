package models

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	openRouterModelsURL    = "https://openrouter.ai/api/v1/models"
	openRouterModelsURLEnv = "TOKENGUARD_OPENROUTER_MODELS_URL"
	pricingSyncOpenRouterEnv = "TOKENGUARD_PRICING_SYNC_OPENROUTER"
	pricingSyncIntervalEnv   = "TOKENGUARD_PRICING_SYNC_INTERVAL"
	defaultPricingSyncInterval = 6 * time.Hour
)

// PricingSyncOpenRouterEnabled reports whether OpenRouter catalog sync should run.
// When the env is unset, defaults to true (recommended for guarded mode).
// Explicit "false" / "0" disables sync.
func PricingSyncOpenRouterEnabled() bool {
	raw := strings.TrimSpace(os.Getenv(pricingSyncOpenRouterEnv))
	if raw == "" {
		return true
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// PricingSyncInterval returns how often to refresh OpenRouter prices in the background.
// Empty env → 6h. "0" → boot-only (no periodic refresh).
func PricingSyncInterval() time.Duration {
	raw := strings.TrimSpace(os.Getenv(pricingSyncIntervalEnv))
	if raw == "" {
		return defaultPricingSyncInterval
	}
	if raw == "0" {
		return 0
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return defaultPricingSyncInterval
	}
	return d
}

// OpenRouterModelPrice is one imported model rate from OpenRouter's public catalog.
type OpenRouterModelPrice struct {
	ModelKey                string
	InputCostPer1KMicroUSD  int64
	OutputCostPer1KMicroUSD int64
}

func openRouterModelsEndpoint() string {
	if raw := strings.TrimSpace(os.Getenv(openRouterModelsURLEnv)); raw != "" {
		return raw
	}
	return openRouterModelsURL
}

// FetchOpenRouterPrices downloads live OpenRouter model pricing (USD/token → micro-USD/1K).
// For each model id "vendor/name" it returns keys: id, openrouter/id, and bare name when useful.
// Override the URL with TOKENGUARD_OPENROUTER_MODELS_URL (used by offline tests).
func FetchOpenRouterPrices(ctx context.Context) ([]OpenRouterModelPrice, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openRouterModelsEndpoint(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter models request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openrouter returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing *struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode openrouter models: %w", err)
	}

	out := make([]OpenRouterModelPrice, 0, len(payload.Data)*2)
	seen := map[string]struct{}{}
	add := func(key string, in, outCost int64) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, OpenRouterModelPrice{
			ModelKey:                key,
			InputCostPer1KMicroUSD:  in,
			OutputCostPer1KMicroUSD: outCost,
		})
	}

	for _, m := range payload.Data {
		if m.ID == "" || m.Pricing == nil {
			continue
		}
		inPer1K, err1 := OpenRouterUSDPerTokenToMicroPer1K(m.Pricing.Prompt)
		outPer1K, err2 := OpenRouterUSDPerTokenToMicroPer1K(m.Pricing.Completion)
		if err1 != nil || err2 != nil {
			continue
		}
		add(m.ID, inPer1K, outPer1K)
		add("openrouter/"+m.ID, inPer1K, outPer1K)
		// Also index leaf name so openai/gpt-4o-mini resolves under provider=openai.
		if i := strings.LastIndexByte(m.ID, '/'); i >= 0 && i+1 < len(m.ID) {
			leaf := m.ID[i+1:]
			add(leaf, inPer1K, outPer1K)
		}
	}
	return out, nil
}
