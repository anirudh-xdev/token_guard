package models

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Price is stored internally as micro-USD per 1K tokens for integer budget math.
// Human-facing APIs and pricing.json prefer USD per 1M tokens (industry standard).
//
// Conversion: $X per 1M tokens = X * 1000 micro-USD per 1K tokens.
// Example: $2.50 / 1M input → input_cost_per_1k = 2500.
type Price struct {
	InputCostPer1KMicroUSD  int64 `json:"-"`
	OutputCostPer1KMicroUSD int64 `json:"-"`
}

type priceJSON struct {
	InputCostPer1K      *int64   `json:"input_cost_per_1k,omitempty"`
	OutputCostPer1K     *int64   `json:"output_cost_per_1k,omitempty"`
	InputUSDPerMillion  *float64 `json:"input_usd_per_million,omitempty"`
	OutputUSDPerMillion *float64 `json:"output_usd_per_million,omitempty"`
}

func (p Price) MarshalJSON() ([]byte, error) {
	inM := MicroPer1KToUSDPerMillion(p.InputCostPer1KMicroUSD)
	outM := MicroPer1KToUSDPerMillion(p.OutputCostPer1KMicroUSD)
	in := p.InputCostPer1KMicroUSD
	out := p.OutputCostPer1KMicroUSD
	return json.Marshal(priceJSON{
		InputCostPer1K:      &in,
		OutputCostPer1K:     &out,
		InputUSDPerMillion:  &inM,
		OutputUSDPerMillion: &outM,
	})
}

func (p *Price) UnmarshalJSON(data []byte) error {
	var raw priceJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	in, err := resolveCostSide(raw.InputCostPer1K, raw.InputUSDPerMillion, "input")
	if err != nil {
		return err
	}
	out, err := resolveCostSide(raw.OutputCostPer1K, raw.OutputUSDPerMillion, "output")
	if err != nil {
		return err
	}
	p.InputCostPer1KMicroUSD = in
	p.OutputCostPer1KMicroUSD = out
	return nil
}

func resolveCostSide(microPer1K *int64, usdPerMillion *float64, side string) (int64, error) {
	if microPer1K != nil {
		if *microPer1K < 0 {
			return 0, fmt.Errorf("%s cost_per_1k cannot be negative", side)
		}
		return *microPer1K, nil
	}
	if usdPerMillion != nil {
		if *usdPerMillion < 0 {
			return 0, fmt.Errorf("%s usd_per_million cannot be negative", side)
		}
		return USDPerMillionToMicroPer1K(*usdPerMillion), nil
	}
	return 0, nil
}

// USDPerMillionToMicroPer1K converts $ per 1M tokens → micro-USD per 1K tokens.
func USDPerMillionToMicroPer1K(usdPerMillion float64) int64 {
	if usdPerMillion == 0 {
		return 0
	}
	return int64(math.Round(usdPerMillion * 1000))
}

// MicroPer1KToUSDPerMillion converts micro-USD per 1K tokens → $ per 1M tokens.
func MicroPer1KToUSDPerMillion(microPer1K int64) float64 {
	return float64(microPer1K) / 1000.0
}

// USDToMicroUSD converts a dollar amount to micro-USD (ceil so we never under-charge).
func USDToMicroUSD(usd float64) int64 {
	if usd <= 0 {
		return 0
	}
	return int64(math.Ceil(usd * 1_000_000))
}

// OpenRouterUSDPerTokenToMicroPer1K converts OpenRouter's USD-per-token quote to micro-USD/1K.
func OpenRouterUSDPerTokenToMicroPer1K(raw string) (int64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, fmt.Errorf("negative price")
	}
	return int64(math.Round(v * 1000 * 1_000_000)), nil
}
