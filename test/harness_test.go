package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
	"tokenguard/internal/proxy"
	"tokenguard/internal/ui"
)

const adminSecret = "tokenguard-e2e-admin-secret"

type harnessOpts struct {
	mgmtEnabled    bool
	guardEnabled   bool
	maxRequestBytes int64
	budgetLimit    int64
	breaker        proxy.LoopBreaker
	upstream       http.Handler
	openaiURL      string
	anthropicURL   string
	openrouterURL  string
}

type harness struct {
	t          *testing.T
	server     *httptest.Server
	store      *memoryStore
	pricing    *models.PricingEngine
	admin      string
	apiKey     string
	userID     string
	events     chan billing.UsageEvent
}

func fixturePath(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("testdata", name)
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "testdata"))
	return filepath.Join(root, name)
}

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(fixturePath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return raw
}

func loadPricingEngine(t *testing.T) *models.PricingEngine {
	t.Helper()
	engine, err := models.LoadPricingFile(context.Background(), fixturePath("pricing.seed.json"))
	if err != nil {
		t.Fatalf("load pricing seed: %v", err)
	}
	return engine
}

func newHarness(t *testing.T, opts harnessOpts) *harness {
	t.Helper()
	if opts.maxRequestBytes == 0 {
		opts.maxRequestBytes = 4 << 20
	}
	if opts.budgetLimit == 0 {
		opts.budgetLimit = 1_000_000 // $1
	}

	upstream := opts.upstream
	if upstream == nil {
		upstream = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(loadFixture(t, "chat.response.json"))
		})
	}
	mock := httptest.NewServer(upstream)
	t.Cleanup(mock.Close)

	openaiURL := opts.openaiURL
	if openaiURL == "" {
		openaiURL = mock.URL
	}
	anthropicURL := opts.anthropicURL
	if anthropicURL == "" {
		anthropicURL = mock.URL
	}
	openrouterURL := opts.openrouterURL
	if openrouterURL == "" {
		openrouterURL = mock.URL
	}

	events := make(chan billing.UsageEvent, 16)
	store := newMemoryStore(events)
	pricing := loadPricingEngine(t)
	seedStoreFromPricing(store, pricing)

	var breaker proxy.LoopBreaker = fixedBreaker{}
	if opts.breaker != nil {
		breaker = opts.breaker
	}

	cfg := proxy.Config{
		ListenAddr:             ":0",
		UpstreamURL:            openaiURL,
		DefaultProvider:        "openai",
		ProviderRoutes: map[string]string{
			"openai":     openaiURL,
			"anthropic":  anthropicURL,
			"openrouter": openrouterURL,
		},
		TokenizerModel:         "gpt-4",
		GuardEnabled:           opts.guardEnabled,
		ManagementEnabled:      opts.mgmtEnabled,
		DefaultMaxOutputTokens: 4096,
		MaxRequestBytes:        opts.maxRequestBytes,
		AdminSecret:            adminSecret,
	}

	var handlerOpts []proxy.HandlerOption
	if opts.guardEnabled {
		handlerOpts = append(handlerOpts, proxy.WithGuard(store, pricing, breaker), proxy.WithAsyncLogTimeout(time.Second))
	}

	handler, err := proxy.NewHandler(cfg, handlerOpts...)
	if err != nil {
		t.Fatalf("NewHandler: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(ui.DocsHTML)
	})
	mux.HandleFunc("/v1/tokenguard.json", handler.HandleDevInfo)
	if opts.mgmtEnabled {
		mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(ui.DashboardHTML)
		})
		mux.HandleFunc("/mgmt/provision", handler.HandleProvision)
		mux.HandleFunc("/mgmt/budget", handler.HandleUpdateBudget)
		mux.HandleFunc("/mgmt/users", handler.HandleListUsers)
		mux.HandleFunc("/mgmt/usage", handler.HandleListUsage)
		mux.HandleFunc("/mgmt/pricing", handler.HandleListPricing)
		mux.HandleFunc("/mgmt/pricing/upsert", handler.HandleUpsertPricing)
		mux.HandleFunc("/mgmt/pricing/delete", handler.HandleDeletePricing)
		mux.HandleFunc("/mgmt/pricing/sync/openrouter", handler.HandleSyncOpenRouterPricing)
	}
	mux.Handle("/", handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	h := &harness{
		t:       t,
		server:  srv,
		store:   store,
		pricing: pricing,
		admin:   adminSecret,
		events:  events,
	}

	if opts.guardEnabled {
		userID, err := store.CreateUser(context.Background(), "seed@example.com", "Seed", opts.budgetLimit)
		if err != nil {
			t.Fatalf("seed user: %v", err)
		}
		_, key, err := store.CreateAPIKey(context.Background(), userID, "default")
		if err != nil {
			t.Fatalf("seed key: %v", err)
		}
		h.userID = userID
		h.apiKey = key
	}
	return h
}

func seedStoreFromPricing(store *memoryStore, pricing *models.PricingEngine) {
	for key, price := range pricing.Snapshot() {
		_ = store.UpsertModelPrice(context.Background(), billing.ModelPrice{
			ModelKey:        key,
			InputCostPer1K:  price.InputCostPer1KMicroUSD,
			OutputCostPer1K: price.OutputCostPer1KMicroUSD,
		})
	}
}

func (h *harness) url(path string) string {
	return h.server.URL + path
}

func (h *harness) do(method, path string, body []byte, headers map[string]string) (int, []byte, http.Header) {
	h.t.Helper()
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, h.url(path), rdr)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := h.server.Client().Do(req)
	if err != nil {
		h.t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		h.t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw, resp.Header
}

func (h *harness) doJSON(method, path string, body any, headers map[string]string) (int, map[string]any, http.Header) {
	h.t.Helper()
	var rawBody []byte
	if body != nil {
		switch v := body.(type) {
		case []byte:
			rawBody = v
		case string:
			rawBody = []byte(v)
		default:
			var err error
			rawBody, err = json.Marshal(v)
			if err != nil {
				h.t.Fatalf("marshal: %v", err)
			}
		}
	}
	if headers == nil {
		headers = map[string]string{}
	}
	if _, ok := headers["Content-Type"]; !ok && rawBody != nil {
		headers["Content-Type"] = "application/json"
	}
	status, raw, hdr := h.do(method, path, rawBody, headers)
	out := map[string]any{}
	if len(raw) > 0 && (raw[0] == '{' || raw[0] == '[') {
		_ = json.Unmarshal(raw, &out)
		// Also try generic decode into map for nested objects - if array, leave empty and store via _raw
		if len(out) == 0 {
			var arr any
			if err := json.Unmarshal(raw, &arr); err == nil {
				out["_raw"] = arr
			}
		}
	} else if len(raw) > 0 {
		out["_text"] = string(raw)
	}
	// Prefer proper map decode
	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err == nil {
		out = asMap
	}
	return status, out, hdr
}

func (h *harness) adminHeaders() map[string]string {
	return map[string]string{
		"Content-Type":                "application/json",
		"X-TokenGuard-Admin-Secret":   h.admin,
	}
}

func (h *harness) proxyHeaders(extra map[string]string) map[string]string {
	hdr := map[string]string{
		"Content-Type":           "application/json",
		"Authorization":          "Bearer sk-test",
		"X-TokenGuard-API-Key":   h.apiKey,
		"X-TokenGuard-Provider":  "openai",
	}
	for k, v := range extra {
		hdr[k] = v
	}
	return hdr
}

func (h *harness) waitUsage(timeout time.Duration) billing.UsageEvent {
	h.t.Helper()
	select {
	case ev := <-h.events:
		return ev
	case <-time.After(timeout):
		h.t.Fatal("timed out waiting for usage event")
		return billing.UsageEvent{}
	}
}

func (h *harness) drainUsage(timeout time.Duration) {
	deadline := time.After(timeout)
	for {
		select {
		case <-h.events:
		case <-deadline:
			return
		default:
			time.Sleep(5 * time.Millisecond)
			select {
			case <-deadline:
				return
			default:
			}
		}
	}
}

// --- memory store ---

type memoryStore struct {
	mu      sync.Mutex
	users   map[string]billing.UserBudgetView
	keys    map[string]billing.APIKey // plaintext -> key
	budgets map[string]billing.Budget
	prices  map[string]billing.ModelPrice
	usage   []billing.UsageEvent
	events  chan billing.UsageEvent
	failLookup bool
	failBudget bool
	seq     int
}

func newMemoryStore(events chan billing.UsageEvent) *memoryStore {
	return &memoryStore{
		users:   map[string]billing.UserBudgetView{},
		keys:    map[string]billing.APIKey{},
		budgets: map[string]billing.Budget{},
		prices:  map[string]billing.ModelPrice{},
		events:  events,
	}
}

func (s *memoryStore) nextID(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s_%d", prefix, s.seq)
}

func (s *memoryStore) LookupAPIKey(ctx context.Context, plaintextKey string) (billing.APIKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failLookup {
		return billing.APIKey{}, fmt.Errorf("store unavailable")
	}
	key, ok := s.keys[plaintextKey]
	if !ok {
		return billing.APIKey{}, billing.ErrAPIKeyNotFound
	}
	return key, nil
}

func (s *memoryStore) GetUserBudget(ctx context.Context, userID string) (billing.Budget, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failBudget {
		return billing.Budget{}, fmt.Errorf("store unavailable")
	}
	b, ok := s.budgets[userID]
	if !ok {
		return billing.Budget{}, billing.ErrBudgetNotFound
	}
	return b, nil
}

func (s *memoryStore) ReserveBudget(ctx context.Context, userID string, amountMicroUSD int64) (billing.Budget, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.budgets[userID]
	if !ok {
		return billing.Budget{}, false, billing.ErrBudgetNotFound
	}
	if amountMicroUSD > b.AvailableMicroUSD() {
		return b, false, nil
	}
	b.ReservedMicroUSD += amountMicroUSD
	s.budgets[userID] = b
	return b, true, nil
}

func (s *memoryStore) RecordUsage(ctx context.Context, event billing.UsageEvent) error {
	s.mu.Lock()
	s.usage = append(s.usage, event)
	s.mu.Unlock()
	select {
	case s.events <- event:
	default:
	}
	return nil
}

func (s *memoryStore) SettleReservedUsage(ctx context.Context, event billing.UsageEvent, reservedMicroUSD int64) error {
	s.mu.Lock()
	if b, ok := s.budgets[event.UserID]; ok {
		b.ReservedMicroUSD -= reservedMicroUSD
		if b.ReservedMicroUSD < 0 {
			b.ReservedMicroUSD = 0
		}
		if event.Status == "completed" {
			b.SpentMicroUSD += event.ActualCostMicroUSD
		}
		s.budgets[event.UserID] = b
		if u, ok := s.users[event.UserID]; ok {
			u.SpentMicroUSD = b.SpentMicroUSD
			u.LimitMicroUSD = b.LimitMicroUSD
			s.users[event.UserID] = u
		}
	}
	s.usage = append(s.usage, event)
	s.mu.Unlock()
	select {
	case s.events <- event:
	default:
	}
	return nil
}

func (s *memoryStore) ReleaseReservation(ctx context.Context, userID string, reservedMicroUSD int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.budgets[userID]
	if !ok {
		return billing.ErrBudgetNotFound
	}
	b.ReservedMicroUSD -= reservedMicroUSD
	if b.ReservedMicroUSD < 0 {
		b.ReservedMicroUSD = 0
	}
	s.budgets[userID] = b
	return nil
}

func (s *memoryStore) CreateUser(ctx context.Context, email, name string, limitMicroUSD int64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limitMicroUSD <= 0 {
		limitMicroUSD = 1_000_000
	}
	id := s.nextID("user")
	s.users[id] = billing.UserBudgetView{
		UserID:        id,
		Email:         email,
		Name:          name,
		LimitMicroUSD: limitMicroUSD,
	}
	s.budgets[id] = billing.Budget{UserID: id, LimitMicroUSD: limitMicroUSD}
	return id, nil
}

func (s *memoryStore) CreateAPIKey(ctx context.Context, userID, name string) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.nextID("key")
	plain := "tg_" + id
	s.keys[plain] = billing.APIKey{ID: id, UserID: userID, KeyPrefix: plain[:6]}
	return id, plain, nil
}

func (s *memoryStore) UpdateUserBudget(ctx context.Context, userID string, limitMicroUSD int64, resetSpent bool) (billing.UserBudgetView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok {
		return billing.UserBudgetView{}, fmt.Errorf("user not found: %s", userID)
	}
	b := s.budgets[userID]
	b.LimitMicroUSD = limitMicroUSD
	if resetSpent {
		b.SpentMicroUSD = 0
	}
	s.budgets[userID] = b
	u.LimitMicroUSD = limitMicroUSD
	u.SpentMicroUSD = b.SpentMicroUSD
	s.users[userID] = u
	return u, nil
}

func (s *memoryStore) ListUsers(ctx context.Context) ([]billing.UserBudgetView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]billing.UserBudgetView, 0, len(s.users))
	for _, u := range s.users {
		if b, ok := s.budgets[u.UserID]; ok {
			u.LimitMicroUSD = b.LimitMicroUSD
			u.SpentMicroUSD = b.SpentMicroUSD
		}
		out = append(out, u)
	}
	return out, nil
}

func (s *memoryStore) ListRecentUsage(ctx context.Context, limit int) ([]billing.UsageEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > len(s.usage) {
		limit = len(s.usage)
	}
	start := len(s.usage) - limit
	if start < 0 {
		start = 0
	}
	out := make([]billing.UsageEvent, limit)
	copy(out, s.usage[start:])
	return out, nil
}

func (s *memoryStore) ListModelPrices(ctx context.Context) ([]billing.ModelPrice, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]billing.ModelPrice, 0, len(s.prices))
	for _, p := range s.prices {
		p.InputUSDPerMillion = models.MicroPer1KToUSDPerMillion(p.InputCostPer1K)
		p.OutputUSDPerMillion = models.MicroPer1KToUSDPerMillion(p.OutputCostPer1K)
		out = append(out, p)
	}
	return out, nil
}

func (s *memoryStore) UpsertModelPrice(ctx context.Context, price billing.ModelPrice) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if price.ModelKey == "" {
		return fmt.Errorf("model_key is required")
	}
	if price.InputCostPer1K < 0 || price.OutputCostPer1K < 0 {
		return fmt.Errorf("costs cannot be negative")
	}
	s.prices[price.ModelKey] = price
	return nil
}

func (s *memoryStore) DeleteModelPrice(ctx context.Context, modelKey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.prices[modelKey]; !ok {
		return fmt.Errorf("model price not found: %s", modelKey)
	}
	delete(s.prices, modelKey)
	return nil
}

func (s *memoryStore) CountModelPrices(ctx context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.prices)), nil
}

func (s *memoryStore) SeedModelPrices(ctx context.Context, prices map[string]billing.ModelPrice) (int, error) {
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

func (s *memoryStore) UpsertMissingModelPrices(ctx context.Context, prices map[string]billing.ModelPrice) (int, error) {
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

type fixedBreaker struct {
	tripped bool
	err     error
}

func (b fixedBreaker) Check(ctx context.Context, sessionID string, payload []byte) (cache.CircuitBreakerResult, error) {
	if b.err != nil {
		return cache.CircuitBreakerResult{}, b.err
	}
	return cache.CircuitBreakerResult{Tripped: b.tripped}, nil
}

// countingBreaker mirrors Redis loop semantics: same session+payload increments a counter;
// trip when count >= threshold (default 3).
type countingBreaker struct {
	mu        sync.Mutex
	threshold int64
	counts    map[string]int64
}

func newCountingBreaker(threshold int64) *countingBreaker {
	if threshold <= 0 {
		threshold = 3
	}
	return &countingBreaker{threshold: threshold, counts: map[string]int64{}}
}

func (b *countingBreaker) Check(ctx context.Context, sessionID string, payload []byte) (cache.CircuitBreakerResult, error) {
	key := cache.HashText(sessionID) + ":" + cache.HashBytes(payload)
	b.mu.Lock()
	b.counts[key]++
	count := b.counts[key]
	b.mu.Unlock()
	return cache.CircuitBreakerResult{
		Count:     count,
		Threshold: b.threshold,
		Tripped:   count >= b.threshold,
	}, nil
}

// hitCountingUpstream wraps a handler and counts how many times upstream was reached.
type hitCountingUpstream struct {
	mu   sync.Mutex
	hits int
	next http.Handler
}

func (u *hitCountingUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.hits++
	u.mu.Unlock()
	u.next.ServeHTTP(w, r)
}

func (u *hitCountingUpstream) Hits() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.hits
}

// Ensure interfaces compile.
var (
	_ proxy.BudgetStore = (*memoryStore)(nil)
	_ proxy.LoopBreaker = fixedBreaker{}
	_ proxy.LoopBreaker = (*countingBreaker)(nil)
)
