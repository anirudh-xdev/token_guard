package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
)

type guardContext struct {
	apiKey           billing.APIKey
	budget           billing.Budget
	analysis         requestAnalysis
	estimate         models.CostEstimate
	reservedMicroUSD int64
	spendTeamID      string
}

type budgetCheckResult struct {
	apiKey     billing.APIKey
	budget     billing.Budget
	estimate   models.CostEstimate
	affordable bool
	reserved   int64
	err        error
}

type loopCheckResult struct {
	result  cache.CircuitBreakerResult
	err     error
	skipped bool
}

func (h *Handler) guardEnabled() bool {
	return h.budgetStore != nil && h.pricing != nil
}

func (h *Handler) preflight(w http.ResponseWriter, r *http.Request) (*guardContext, bool) {
	apiKeySecret := tokenGuardAPIKey(r)
	if apiKeySecret == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "TokenGuard: missing X-TokenGuard-API-Key",
			"code":  "missing_api_key",
		})
		return nil, false
	}

	body, err := readRequestBody(r, h.maxRequestBytes)
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{
			"error": "TokenGuard: request body is too large",
			"code":  "request_too_large",
		})
		return nil, false
	}
	analysis, err := analyzeRequest(r, body, h.tokenEncoder, h.defaultMaxOutputTokens)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "TokenGuard: " + err.Error(),
			"code":  "bad_request",
		})
		return nil, false
	}

	spendTeamID := strings.TrimSpace(r.Header.Get(tokenGuardTeamIDHeader))
	budgetCtx := r.Context()
	if spendTeamID != "" {
		budgetCtx = billing.WithSpendTeamID(budgetCtx, spendTeamID)
	}

	budgetCh := make(chan budgetCheckResult, 1)
	loopCh := make(chan loopCheckResult, 1)

	go func() {
		budgetCh <- h.checkBudget(budgetCtx, apiKeySecret, analysis)
	}()
	go func() {
		loopCh <- h.checkLoop(r.Context(), analysis)
	}()

	budgetResult := <-budgetCh
	if budgetResult.err != nil {
		if budgetResult.apiKey.UserID != "" {
			h.logUsageAsync(billing.UsageEvent{
				UserID:                budgetResult.apiKey.UserID,
				APIKeyID:              budgetResult.apiKey.ID,
				Provider:              analysis.Provider,
				Model:                 modelOrUnknown(analysis.Model),
				SessionID:             analysis.SessionID,
				InputTokens:           analysis.InputTokens,
				EstimatedCostMicroUSD: budgetResult.estimate.EstimatedTotalCostMicroUSD,
				Status:                "blocked_budget",
			})
		}
		h.handleBudgetError(w, budgetResult.err)
		return nil, false
	}

	loopResult := <-loopCh
	if loopResult.err != nil {
		h.releaseReservationAsync(budgetResult.apiKey.UserID, budgetResult.reserved, spendTeamID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "TokenGuard: circuit breaker unavailable",
			"code":  "loop_check_unavailable",
		})
		h.logUsageAsync(billing.UsageEvent{
			UserID:                budgetResult.apiKey.UserID,
			APIKeyID:              budgetResult.apiKey.ID,
			Provider:              analysis.Provider,
			Model:                 modelOrUnknown(analysis.Model),
			SessionID:             analysis.SessionID,
			InputTokens:           analysis.InputTokens,
			EstimatedCostMicroUSD: budgetResult.estimate.EstimatedTotalCostMicroUSD,
			Status:                "blocked_loop",
		})
		return nil, false
	}
	if !loopResult.skipped && loopResult.result.Tripped {
		w.Header().Set("X-TokenGuard-Loop-Check", fmt.Sprintf("tripped;count=%d;threshold=%d", loopResult.result.Count, loopResult.result.Threshold))
		h.releaseReservationAsync(budgetResult.apiKey.UserID, budgetResult.reserved, spendTeamID)
		writeJSON(w, http.StatusConflict, map[string]string{
			"error": "TokenGuard: Infinite agent loop detected. Circuit breaker tripped to save budget.",
			"code":  "loop_detected",
		})
		h.logUsageAsync(billing.UsageEvent{
			UserID:                budgetResult.apiKey.UserID,
			APIKeyID:              budgetResult.apiKey.ID,
			Provider:              analysis.Provider,
			Model:                 modelOrUnknown(analysis.Model),
			SessionID:             analysis.SessionID,
			InputTokens:           analysis.InputTokens,
			EstimatedCostMicroUSD: budgetResult.estimate.EstimatedTotalCostMicroUSD,
			Status:                "blocked_loop",
		})
		return nil, false
	}

	if !budgetResult.affordable {
		writeJSON(w, http.StatusPaymentRequired, map[string]any{
			"error":                   "TokenGuard: budget exceeded",
			"code":                    "budget_exceeded",
			"available_microusd":      budgetResult.budget.AvailableMicroUSD(),
			"spent_microusd":          budgetResult.budget.SpentMicroUSD,
			"limit_microusd":          budgetResult.budget.LimitMicroUSD,
			"estimated_cost_microusd": budgetResult.estimate.EstimatedTotalCostMicroUSD,
			"model":                   modelOrUnknown(analysis.Model),
		})
		h.logUsageAsync(billing.UsageEvent{
			UserID:                budgetResult.apiKey.UserID,
			APIKeyID:              budgetResult.apiKey.ID,
			Provider:              analysis.Provider,
			Model:                 modelOrUnknown(analysis.Model),
			SessionID:             analysis.SessionID,
			InputTokens:           analysis.InputTokens,
			EstimatedCostMicroUSD: budgetResult.estimate.EstimatedTotalCostMicroUSD,
			Status:                "blocked_budget",
		})
		return nil, false
	}

	if loopResult.skipped {
		reason := "no_session"
		if analysis.SessionID != "" && len(analysis.SemanticPayload) == 0 {
			reason = "empty_payload"
		} else if h.circuitBreaker == nil {
			reason = "breaker_disabled"
		}
		w.Header().Set("X-TokenGuard-Loop-Check", "skipped;reason="+reason)
	} else {
		w.Header().Set("X-TokenGuard-Loop-Check", fmt.Sprintf("ok;count=%d;threshold=%d", loopResult.result.Count, loopResult.result.Threshold))
	}

	stripTokenGuardHeaders(r)
	restoreRequestBody(r, body)
	return &guardContext{
		apiKey:           budgetResult.apiKey,
		budget:           budgetResult.budget,
		analysis:         analysis,
		estimate:         budgetResult.estimate,
		reservedMicroUSD: budgetResult.reserved,
		spendTeamID:      spendTeamID,
	}, true
}

func (h *Handler) checkBudget(parent context.Context, apiKeySecret string, analysis requestAnalysis) budgetCheckResult {
	// Detach from the inbound request cancel and give Turso a generous deadline.
	// Flaky/slow Turso HTTP was surfacing as budget_unavailable on longer prompts.
	dbCtx, cancel := billing.WithTimeout(parent, h.dbOpTimeout())
	defer cancel()

	var apiKey billing.APIKey
	err := billing.Retry(dbCtx, 3, func(ctx context.Context) error {
		var lookupErr error
		apiKey, lookupErr = h.budgetStore.LookupAPIKey(ctx, apiKeySecret)
		return lookupErr
	})
	if err != nil {
		return budgetCheckResult{err: err}
	}

	result := budgetCheckResult{
		apiKey:     apiKey,
		affordable: true,
	}
	if analysis.Model == "" {
		result.err = errors.New("model is required for budget checks")
		return result
	}

	var budget billing.Budget
	err = billing.Retry(dbCtx, 3, func(ctx context.Context) error {
		var getErr error
		budget, getErr = h.budgetStore.GetUserBudget(ctx, apiKey.UserID)
		return getErr
	})
	if err != nil {
		return budgetCheckResult{apiKey: apiKey, err: err}
	}
	result.budget = budget
	spendTeamID := billing.SpendTeamIDFromContext(parent)

	estimate, err := h.pricing.EstimateProvider(
		analysis.Provider,
		analysis.Model,
		analysis.InputTokens,
		analysis.MaxOutputTokens,
	)
	if err != nil {
		result.err = err
		return result
	}
	result.estimate = estimate

	// Personal budget (default): hard-stop when nothing left, even if estimate is $0.
	if spendTeamID == "" {
		available := budget.AvailableMicroUSD()
		if available <= 0 || estimate.EstimatedTotalCostMicroUSD > available {
			result.affordable = false
			return result
		}
	}

	var reservedBudget billing.Budget
	var reserved bool
	err = billing.Retry(dbCtx, 3, func(ctx context.Context) error {
		var reserveErr error
		reservedBudget, reserved, reserveErr = h.budgetStore.ReserveBudget(ctx, apiKey.UserID, estimate.EstimatedTotalCostMicroUSD)
		return reserveErr
	})
	result.budget = reservedBudget
	result.affordable = reserved
	if reserved {
		result.reserved = estimate.EstimatedTotalCostMicroUSD
	}
	result.err = err
	return result
}

func (h *Handler) dbOpTimeout() time.Duration {
	type timedStore interface {
		OpTimeout() time.Duration
	}
	if s, ok := h.budgetStore.(timedStore); ok {
		if d := s.OpTimeout(); d > 0 {
			return d
		}
	}
	return 15 * time.Second
}

func (h *Handler) checkLoop(ctx context.Context, analysis requestAnalysis) loopCheckResult {
	if h.circuitBreaker == nil || analysis.SessionID == "" || len(analysis.SemanticPayload) == 0 {
		return loopCheckResult{skipped: true}
	}
	result, err := h.circuitBreaker.Check(ctx, analysis.SessionID, analysis.SemanticPayload)
	return loopCheckResult{result: result, err: err}
}

func (h *Handler) handleBudgetError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, billing.ErrAPIKeyNotFound):
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"error": "TokenGuard: invalid API key",
			"code":  "invalid_api_key",
		})
	case errors.Is(err, billing.ErrBudgetNotFound):
		writeJSON(w, http.StatusPaymentRequired, map[string]string{
			"error": "TokenGuard: budget not configured",
			"code":  "budget_not_configured",
		})
	case errors.Is(err, billing.ErrTeamNotFound):
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "TokenGuard: invalid or inaccessible X-TokenGuard-Team-ID",
			"code":  "invalid_team_id",
		})
	case strings.Contains(err.Error(), "model is required"):
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "TokenGuard: model is required for budget checks",
			"code":  "model_required",
		})
	case strings.Contains(err.Error(), "pricing not found"):
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "TokenGuard: model pricing not configured",
			"code":  "pricing_not_configured",
		})
	case errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) ||
		strings.Contains(strings.ToLower(err.Error()), "deadline exceeded") ||
		strings.Contains(strings.ToLower(err.Error()), "timeout"):
		log.Printf("budget check timeout: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "TokenGuard: budget database timed out (Turso unreachable or slow). Retry; check TURSO_* and network.",
			"code":  "budget_timeout",
		})
	default:
		log.Printf("budget check unavailable: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "TokenGuard: budget check unavailable",
			"code":  "budget_unavailable",
		})
	}
}

func (h *Handler) logCompletedUsageAsync(guard *guardContext, streamEvent StreamTokenEvent, statusCode int) {
	status := "completed"
	actualCost := guard.estimate.EstimatedTotalCostMicroUSD
	outputTokens := streamEvent.TotalTokens
	inputTokens := guard.analysis.InputTokens
	if streamEvent.InputTokens > 0 {
		inputTokens = streamEvent.InputTokens
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
		status = "provider_error"
		actualCost = 0
		outputTokens = 0
	} else if streamEvent.HasProviderCost && streamEvent.CostMicroUSD > 0 {
		// Prefer provider-reported USD (OpenRouter usage.cost) — real billed amount.
		actualCost = streamEvent.CostMicroUSD
	} else if guard.analysis.Model != "" && (inputTokens > 0 || outputTokens > 0) {
		calculated, err := h.pricing.ActualCostMicroUSDProvider(guard.analysis.Provider, guard.analysis.Model, inputTokens, outputTokens)
		if err == nil {
			actualCost = calculated
		}
	}

	h.settleUsageAsync(billing.UsageEvent{
		UserID:                guard.apiKey.UserID,
		APIKeyID:              guard.apiKey.ID,
		Provider:              guard.analysis.Provider,
		Model:                 modelOrUnknown(guard.analysis.Model),
		SessionID:             guard.analysis.SessionID,
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		EstimatedCostMicroUSD: guard.estimate.EstimatedTotalCostMicroUSD,
		ActualCostMicroUSD:    actualCost,
		Status:                status,
	}, guard.reservedMicroUSD, guard.spendTeamID)
}

func (h *Handler) logUsageAsync(event billing.UsageEvent) {
	if h.budgetStore == nil || event.UserID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), h.asyncLogTimeout)
		defer cancel()
		if err := h.budgetStore.RecordUsage(ctx, event); err != nil {
			log.Printf("async usage log failed user_id=%s status=%s error=%v", event.UserID, event.Status, err)
		}
	}()
}

func (h *Handler) settleUsageAsync(event billing.UsageEvent, reservedMicroUSD int64, spendTeamID string) {
	if h.budgetStore == nil || event.UserID == "" {
		return
	}
	go func() {
		if err := h.settleUsageWithRetry(event, reservedMicroUSD, spendTeamID); err != nil {
			log.Printf("async reserved usage settlement failed user_id=%s status=%s error=%v", event.UserID, event.Status, err)
			h.releaseReservationSync(event.UserID, reservedMicroUSD, spendTeamID)
		}
	}()
}

func (h *Handler) settleUsageWithRetry(event billing.UsageEvent, reservedMicroUSD int64, spendTeamID string) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), h.asyncLogTimeout)
		ctx = billing.WithSpendTeamID(ctx, spendTeamID)
		err := h.budgetStore.SettleReservedUsage(ctx, event, reservedMicroUSD)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * 100 * time.Millisecond)
		}
	}
	return lastErr
}

func (h *Handler) releaseReservationAsync(userID string, reservedMicroUSD int64, spendTeamID string) {
	if h.budgetStore == nil || userID == "" || reservedMicroUSD <= 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), h.asyncLogTimeout)
		defer cancel()
		ctx = billing.WithSpendTeamID(ctx, spendTeamID)
		if err := h.budgetStore.ReleaseReservation(ctx, userID, reservedMicroUSD); err != nil {
			log.Printf("async reservation release failed user_id=%s error=%v", userID, err)
		}
	}()
}

func (h *Handler) releaseReservationSync(userID string, reservedMicroUSD int64, spendTeamID string) {
	if h.budgetStore == nil || userID == "" || reservedMicroUSD <= 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), h.asyncLogTimeout)
	defer cancel()
	ctx = billing.WithSpendTeamID(ctx, spendTeamID)
	if err := h.budgetStore.ReleaseReservation(ctx, userID, reservedMicroUSD); err != nil {
		log.Printf("reservation release fallback failed user_id=%s error=%v", userID, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeManagementJSON writes JSON for /mgmt/* routes and includes CORS headers
// so the admin dashboard can call the API cross-origin when needed.
func writeManagementJSON(w http.ResponseWriter, status int, payload any) {
	setManagementCORSHeaders(w)
	writeJSON(w, status, payload)
}

func setManagementCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-TokenGuard-Admin-Secret")
}

func writeManagementOptions(w http.ResponseWriter) {
	setManagementCORSHeaders(w)
	w.WriteHeader(http.StatusNoContent)
}

func modelOrUnknown(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "unknown"
	}
	return model
}
