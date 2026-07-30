package proxy

import (
	"context"
	"testing"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
)

type fakeBudgetStore struct {
	apiKey billing.APIKey
	budget billing.Budget
	events chan billing.UsageEvent
}

func newFakeBudgetStore(availableMicroUSD int64) *fakeBudgetStore {
	return &fakeBudgetStore{
		apiKey: billing.APIKey{ID: "key_1", UserID: "user_1", KeyPrefix: "tg_test"},
		budget: billing.Budget{
			UserID:        "user_1",
			LimitMicroUSD: availableMicroUSD,
		},
		events: make(chan billing.UsageEvent, 4),
	}
}

func (s *fakeBudgetStore) LookupAPIKey(ctx context.Context, plaintextKey string) (billing.APIKey, error) {
	if plaintextKey != "tg_test" {
		return billing.APIKey{}, billing.ErrAPIKeyNotFound
	}
	return s.apiKey, nil
}

func (s *fakeBudgetStore) GetUserBudget(ctx context.Context, userID string) (billing.Budget, error) {
	if userID != s.budget.UserID {
		return billing.Budget{}, billing.ErrBudgetNotFound
	}
	return s.budget, nil
}

func (s *fakeBudgetStore) ReserveBudget(ctx context.Context, userID string, amountMicroUSD int64) (billing.Budget, bool, error) {
	if userID != s.budget.UserID {
		return billing.Budget{}, false, billing.ErrBudgetNotFound
	}
	if amountMicroUSD > s.budget.AvailableMicroUSD() {
		return s.budget, false, nil
	}
	s.budget.ReservedMicroUSD += amountMicroUSD
	return s.budget, true, nil
}

func (s *fakeBudgetStore) RecordUsage(ctx context.Context, event billing.UsageEvent) error {
	select {
	case s.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *fakeBudgetStore) SettleReservedUsage(ctx context.Context, event billing.UsageEvent, reservedMicroUSD int64) error {
	if reservedMicroUSD > 0 {
		s.budget.ReservedMicroUSD -= reservedMicroUSD
		if s.budget.ReservedMicroUSD < 0 {
			s.budget.ReservedMicroUSD = 0
		}
		if event.Status == "completed" {
			s.budget.SpentMicroUSD += event.ActualCostMicroUSD
		}
	}
	return s.RecordUsage(ctx, event)
}

func (s *fakeBudgetStore) ReleaseReservation(ctx context.Context, userID string, reservedMicroUSD int64) error {
	if userID != s.budget.UserID {
		return billing.ErrBudgetNotFound
	}
	s.budget.ReservedMicroUSD -= reservedMicroUSD
	if s.budget.ReservedMicroUSD < 0 {
		s.budget.ReservedMicroUSD = 0
	}
	return nil
}

func (s *fakeBudgetStore) CreateUser(ctx context.Context, email, name string, limitMicroUSD int64) (string, error) {
	return "user_created", nil
}

func (s *fakeBudgetStore) CreateAPIKey(ctx context.Context, userID, name string) (string, string, error) {
	return "key_created", "tg_created", nil
}

func (s *fakeBudgetStore) UpdateUserBudget(ctx context.Context, userID string, limitMicroUSD int64, resetSpent bool) (billing.UserBudgetView, error) {
	return billing.UserBudgetView{UserID: userID, LimitMicroUSD: limitMicroUSD}, nil
}

func (s *fakeBudgetStore) ListUsers(ctx context.Context) ([]billing.UserBudgetView, error) {
	return nil, nil
}

func (s *fakeBudgetStore) ListRecentUsage(ctx context.Context, limit int) ([]billing.UsageEvent, error) {
	return nil, nil
}

func (s *fakeBudgetStore) ListModelPrices(ctx context.Context) ([]billing.ModelPrice, error) {
	return nil, nil
}

func (s *fakeBudgetStore) UpsertModelPrice(ctx context.Context, price billing.ModelPrice) error {
	return nil
}

func (s *fakeBudgetStore) DeleteModelPrice(ctx context.Context, modelKey string) error {
	return nil
}

func (s *fakeBudgetStore) CountModelPrices(ctx context.Context) (int64, error) {
	return 0, nil
}

func (s *fakeBudgetStore) SeedModelPrices(ctx context.Context, prices map[string]billing.ModelPrice) (int, error) {
	return 0, nil
}

func (s *fakeBudgetStore) UpsertMissingModelPrices(ctx context.Context, prices map[string]billing.ModelPrice) (int, error) {
	return 0, nil
}

type fakeLoopBreaker struct {
	result cache.CircuitBreakerResult
	err    error
}

func (b fakeLoopBreaker) Check(ctx context.Context, sessionID string, payload []byte) (cache.CircuitBreakerResult, error) {
	return b.result, b.err
}

func mustTestPricing(t *testing.T) *models.PricingEngine {
	t.Helper()
	pricing, err := models.NewPricingEngine(map[string]models.Price{
		"gpt-test": {
			InputCostPer1KMicroUSD:  1000,
			OutputCostPer1KMicroUSD: 1000,
		},
	})
	if err != nil {
		t.Fatalf("NewPricingEngine returned error: %v", err)
	}
	return pricing
}

func waitUsageEvent(t *testing.T, events <-chan billing.UsageEvent) billing.UsageEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for usage event")
		return billing.UsageEvent{}
	}
}
