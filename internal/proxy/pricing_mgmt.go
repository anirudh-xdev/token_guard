package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/models"
)

type upsertPriceRequest struct {
	ModelKey        string `json:"model_key"`
	InputCostPer1K  int64  `json:"input_cost_per_1k"`
	OutputCostPer1K int64  `json:"output_cost_per_1k"`
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
	writeManagementJSON(w, http.StatusOK, map[string]any{"prices": prices, "count": len(prices)})
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
	mp := billing.ModelPrice{
		ModelKey:        req.ModelKey,
		InputCostPer1K:  req.InputCostPer1K,
		OutputCostPer1K: req.OutputCostPer1K,
	}
	if err := h.budgetStore.UpsertModelPrice(r.Context(), mp); err != nil {
		writeManagementJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = h.pricing.Upsert(mp.ModelKey, models.Price{
		InputCostPer1KMicroUSD:  mp.InputCostPer1K,
		OutputCostPer1KMicroUSD: mp.OutputCostPer1K,
	})
	writeManagementJSON(w, http.StatusOK, map[string]any{"price": mp})
}

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

// HandleSyncOpenRouterPricing imports model prices from OpenRouter's public models API.
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

	imported, err := syncOpenRouterPrices(r, h)
	if err != nil {
		writeManagementJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeManagementJSON(w, http.StatusOK, map[string]any{
		"imported":      imported,
		"models_priced": h.pricing.ModelCount(),
	})
}

func syncOpenRouterPrices(r *http.Request, h *Handler) (int, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("openrouter models request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, fmt.Errorf("openrouter returned HTTP %d", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID           string `json:"id"`
			Pricing      *struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return 0, fmt.Errorf("decode openrouter models: %w", err)
	}

	imported := 0
	for _, m := range payload.Data {
		if m.ID == "" || m.Pricing == nil {
			continue
		}
		inPerTok, err1 := parseFloatString(m.Pricing.Prompt)
		outPerTok, err2 := parseFloatString(m.Pricing.Completion)
		if err1 != nil || err2 != nil {
			continue
		}
		// OpenRouter quotes USD per token; convert to micro-USD per 1K tokens.
		inPer1K := int64(math.Round(inPerTok * 1000 * 1_000_000))
		outPer1K := int64(math.Round(outPerTok * 1000 * 1_000_000))
		if inPer1K < 0 || outPer1K < 0 {
			continue
		}
		keys := []string{m.ID, "openrouter/" + m.ID}
		for _, key := range keys {
			mp := billing.ModelPrice{ModelKey: key, InputCostPer1K: inPer1K, OutputCostPer1K: outPer1K}
			if err := h.budgetStore.UpsertModelPrice(r.Context(), mp); err != nil {
				return imported, err
			}
			_ = h.pricing.Upsert(key, models.Price{
				InputCostPer1KMicroUSD:  inPer1K,
				OutputCostPer1KMicroUSD: outPer1K,
			})
			imported++
		}
	}
	return imported, nil
}

func parseFloatString(raw string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("empty")
	}
	var v float64
	_, err := fmt.Sscanf(raw, "%f", &v)
	return v, err
}
