package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/models"
)

type memPriceStore struct {
	mu     sync.Mutex
	prices map[string]billing.ModelPrice
}

func newMemPriceStore() *memPriceStore {
	return &memPriceStore{prices: make(map[string]billing.ModelPrice)}
}

func (s *memPriceStore) LookupAPIKey(ctx context.Context, plaintextKey string) (billing.APIKey, error) {
	return billing.APIKey{}, nil
}
func (s *memPriceStore) GetUserBudget(ctx context.Context, userID string) (billing.Budget, error) {
	return billing.Budget{}, nil
}
func (s *memPriceStore) ReserveBudget(ctx context.Context, userID string, amountMicroUSD int64) (billing.Budget, bool, error) {
	return billing.Budget{}, false, nil
}
func (s *memPriceStore) RecordUsage(ctx context.Context, event billing.UsageEvent) error { return nil }
func (s *memPriceStore) SettleReservedUsage(ctx context.Context, event billing.UsageEvent, reservedMicroUSD int64) error {
	return nil
}
func (s *memPriceStore) ReleaseReservation(ctx context.Context, userID string, reservedMicroUSD int64) error {
	return nil
}
func (s *memPriceStore) CreateUser(ctx context.Context, email, name string, limitMicroUSD int64) (string, error) {
	return "", nil
}
func (s *memPriceStore) CreateAPIKey(ctx context.Context, userID, name string) (string, string, error) {
	return "", "", nil
}
func (s *memPriceStore) UpdateUserBudget(ctx context.Context, userID string, limitMicroUSD int64, resetSpent bool) (billing.UserBudgetView, error) {
	return billing.UserBudgetView{}, nil
}
func (s *memPriceStore) ListUsers(ctx context.Context) ([]billing.UserBudgetView, error) { return nil, nil }
func (s *memPriceStore) ListRecentUsage(ctx context.Context, limit int) ([]billing.UsageEvent, error) {
	return nil, nil
}

func (s *memPriceStore) ListModelPrices(ctx context.Context) ([]billing.ModelPrice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]billing.ModelPrice, 0, len(s.prices))
	for _, p := range s.prices {
		out = append(out, p)
	}
	return out, nil
}

func (s *memPriceStore) UpsertModelPrice(ctx context.Context, price billing.ModelPrice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prices[price.ModelKey] = price
	return nil
}

func (s *memPriceStore) DeleteModelPrice(ctx context.Context, modelKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.prices, modelKey)
	return nil
}

func (s *memPriceStore) CountModelPrices(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.prices)), nil
}

func (s *memPriceStore) SeedModelPrices(ctx context.Context, prices map[string]billing.ModelPrice) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.prices) > 0 {
		return 0, nil
	}
	n := 0
	for k, p := range prices {
		p.ModelKey = k
		s.prices[k] = p
		n++
	}
	return n, nil
}

func (s *memPriceStore) UpsertMissingModelPrices(ctx context.Context, prices map[string]billing.ModelPrice) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for k, p := range prices {
		if _, ok := s.prices[k]; ok {
			continue
		}
		p.ModelKey = k
		s.prices[k] = p
		n++
	}
	return n, nil
}

func TestBootstrapPricingCatalogFromOpenRouterOnly(t *testing.T) {
	body := []byte(`{"data":[{"id":"openai/gpt-boot-sync","pricing":{"prompt":"0.00000015","completion":"0.0000006"}}]}`)
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(or.Close)
	t.Setenv("TOKENGUARD_OPENROUTER_MODELS_URL", or.URL)

	store := newMemPriceStore()
	pricing := models.EmptyPricingEngine()
	err := BootstrapPricingCatalog(context.Background(), store, pricing, BootstrapPricingOptions{SyncOpenRouter: true})
	if err != nil {
		t.Fatalf("BootstrapPricingCatalog: %v", err)
	}
	if pricing.ModelCount() < 1 {
		t.Fatalf("expected synced models, got %d", pricing.ModelCount())
	}
	if _, ok := pricing.PriceForModel("gpt-boot-sync"); !ok {
		t.Fatal("expected leaf model gpt-boot-sync in catalog")
	}
}

func TestBootstrapPricingCatalogFailsWhenEmpty(t *testing.T) {
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(or.Close)
	t.Setenv("TOKENGUARD_OPENROUTER_MODELS_URL", or.URL)

	store := newMemPriceStore()
	pricing := models.EmptyPricingEngine()
	err := BootstrapPricingCatalog(context.Background(), store, pricing, BootstrapPricingOptions{SyncOpenRouter: true})
	if err == nil {
		t.Fatal("expected error when sync fails and catalog empty")
	}
}

func TestBootstrapPricingCatalogKeepsFileWhenSyncFails(t *testing.T) {
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusBadGateway)
	}))
	t.Cleanup(or.Close)
	t.Setenv("TOKENGUARD_OPENROUTER_MODELS_URL", or.URL)

	store := newMemPriceStore()
	pricing, err := models.NewPricingEngine(map[string]models.Price{
		"file-model": {InputCostPer1KMicroUSD: 100, OutputCostPer1KMicroUSD: 200},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := BootstrapPricingCatalog(context.Background(), store, pricing, BootstrapPricingOptions{SyncOpenRouter: true}); err != nil {
		t.Fatalf("BootstrapPricingCatalog: %v", err)
	}
	if _, ok := pricing.PriceForModel("file-model"); !ok {
		t.Fatal("file-model should remain after failed sync")
	}
}

func TestRunPricingSyncLoopAddsModel(t *testing.T) {
	body := []byte(`{"data":[{"id":"openai/gpt-interval","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`)
	or := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(or.Close)
	t.Setenv("TOKENGUARD_OPENROUTER_MODELS_URL", or.URL)

	store := newMemPriceStore()
	pricing, err := models.NewPricingEngine(map[string]models.Price{
		"seed": {InputCostPer1KMicroUSD: 1, OutputCostPer1KMicroUSD: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = store.UpsertModelPrice(context.Background(), billing.ModelPrice{ModelKey: "seed", InputCostPer1K: 1, OutputCostPer1K: 1})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go RunPricingSyncLoop(ctx, store, pricing, 20*time.Millisecond)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := pricing.PriceForModel("gpt-interval"); ok {
			cancel()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timed out waiting for interval sync to add gpt-interval")
}
