package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"tokenguard/internal/billing"
	"tokenguard/internal/models"
)

type upsertPriceRequest struct {
	ModelKey            string   `json:"model_key"`
	InputCostPer1K      *int64   `json:"input_cost_per_1k"`
	OutputCostPer1K     *int64   `json:"output_cost_per_1k"`
	InputUSDPerMillion  *float64 `json:"input_usd_per_million"`
	OutputUSDPerMillion *float64 `json:"output_usd_per_million"`
}

type deletePriceRequest struct {
	ModelKey string `json:"model_key"`
}

func (h *Handler) HandleListPricing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.authorizedManagementRequest(r) {
		writeManagementJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized management access"})
		return
	}
	if h.budgetStore == nil {
		writeManagementJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Management store unavailable"})
		return
	}
	prices, err := h.budgetStore.ListModelPrices(r.Context())
	if err != nil {
		writeManagementJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list pricing: " + err.Error()})
		return
	}
	writeManagementJSON(w, http.StatusOK, map[string]any{
		"prices": prices,
		"count":  len(prices),
		"unit":   "Prefer input_usd_per_million / output_usd_per_million ($ per 1M tokens). Legacy input_cost_per_1k is micro-USD per 1K.",
	})
}

func (h *Handler) HandleUpsertPricing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.authorizedManagementRequest(r) {
		writeManagementJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized management access"})
		return
	}
	if h.budgetStore == nil || h.pricing == nil {
		writeManagementJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Pricing store unavailable"})
		return
	}

	var req upsertPriceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	in, out, err := resolvePriceSides(req)
	if err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	mp := billing.ModelPrice{
		ModelKey:        req.ModelKey,
		InputCostPer1K:  in,
		OutputCostPer1K: out,
	}
	if err := h.budgetStore.UpsertModelPrice(r.Context(), mp); err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = h.pricing.Upsert(mp.ModelKey, models.Price{
		InputCostPer1KMicroUSD:  mp.InputCostPer1K,
		OutputCostPer1KMicroUSD: mp.OutputCostPer1K,
	})
	mp.InputUSDPerMillion = models.MicroPer1KToUSDPerMillion(mp.InputCostPer1K)
	mp.OutputUSDPerMillion = models.MicroPer1KToUSDPerMillion(mp.OutputCostPer1K)
	writeManagementJSON(w, http.StatusOK, map[string]any{"price": mp})
}

func resolvePriceSides(req upsertPriceRequest) (in, out int64, err error) {
	switch {
	case req.InputUSDPerMillion != nil:
		if *req.InputUSDPerMillion < 0 {
			return 0, 0, errNegativePrice
		}
		in = models.USDPerMillionToMicroPer1K(*req.InputUSDPerMillion)
	case req.InputCostPer1K != nil:
		if *req.InputCostPer1K < 0 {
			return 0, 0, errNegativePrice
		}
		in = *req.InputCostPer1K
	default:
		return 0, 0, errPriceRequired
	}
	switch {
	case req.OutputUSDPerMillion != nil:
		if *req.OutputUSDPerMillion < 0 {
			return 0, 0, errNegativePrice
		}
		out = models.USDPerMillionToMicroPer1K(*req.OutputUSDPerMillion)
	case req.OutputCostPer1K != nil:
		if *req.OutputCostPer1K < 0 {
			return 0, 0, errNegativePrice
		}
		out = *req.OutputCostPer1K
	default:
		return 0, 0, errPriceRequired
	}
	return in, out, nil
}

var (
	errNegativePrice = jsonError("costs cannot be negative")
	errPriceRequired = jsonError("provide input_usd_per_million/output_usd_per_million (preferred) or input_cost_per_1k/output_cost_per_1k")
)

func (h *Handler) HandleDeletePricing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.authorizedManagementRequest(r) {
		writeManagementJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized management access"})
		return
	}
	if h.budgetStore == nil || h.pricing == nil {
		writeManagementJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Pricing store unavailable"})
		return
	}

	var req deletePriceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if err := h.budgetStore.DeleteModelPrice(r.Context(), req.ModelKey); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeManagementJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	h.pricing.Delete(req.ModelKey)
	writeManagementJSON(w, http.StatusOK, map[string]string{"deleted": strings.TrimSpace(req.ModelKey)})
}

// HandleSyncOpenRouterPricing imports live model prices from OpenRouter's public models API.
func (h *Handler) HandleSyncOpenRouterPricing(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		writeManagementOptions(w)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !h.authorizedManagementRequest(r) {
		writeManagementJSON(w, http.StatusUnauthorized, map[string]string{"error": "Unauthorized management access"})
		return
	}
	if h.budgetStore == nil || h.pricing == nil {
		writeManagementJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Pricing store unavailable"})
		return
	}

	imported, err := syncOpenRouterIntoStore(r.Context(), h.budgetStore, h.pricing)
	if err != nil {
		writeManagementJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeManagementJSON(w, http.StatusOK, map[string]any{
		"imported":      imported,
		"models_priced": h.pricing.ModelCount(),
		"source":        "https://openrouter.ai/api/v1/models",
		"note":          "Live OpenRouter USD rates; stored as micro-USD/1K and exposed as usd_per_million.",
	})
}

func syncOpenRouterIntoStore(ctx context.Context, store BudgetStore, pricing *models.PricingEngine) (int, error) {
	fetched, err := models.FetchOpenRouterPrices(ctx)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, row := range fetched {
		mp := billing.ModelPrice{
			ModelKey:        row.ModelKey,
			InputCostPer1K:  row.InputCostPer1KMicroUSD,
			OutputCostPer1K: row.OutputCostPer1KMicroUSD,
		}
		if err := store.UpsertModelPrice(ctx, mp); err != nil {
			return imported, err
		}
		_ = pricing.Upsert(row.ModelKey, models.Price{
			InputCostPer1KMicroUSD:  row.InputCostPer1KMicroUSD,
			OutputCostPer1KMicroUSD: row.OutputCostPer1KMicroUSD,
		})
		imported++
	}
	return imported, nil
}
