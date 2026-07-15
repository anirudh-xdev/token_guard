package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
)

func TestHandlerForwardsRequestToUpstream(t *testing.T) {
	var upstreamHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("path = %q, want /v1/chat/completions", r.URL.Path)
		}
		if r.URL.RawQuery != "stream=true" {
			t.Fatalf("query = %q, want stream=true", r.URL.RawQuery)
		}
		if r.Host != upstreamHost {
			t.Fatalf("Host = %q, want %q", r.Host, upstreamHost)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Fatalf("Authorization = %q, want bearer token", got)
		}
		if got := r.Header.Get("X-Forwarded-Host"); got == "" {
			t.Fatal("X-Forwarded-Host was not set")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if string(body) != `{"model":"gpt-test"}` {
			t.Fatalf("body = %q, want request body preserved", string(body))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream URL: %v", err)
	}
	upstreamHost = upstreamURL.Host

	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: upstream.URL,
	}, withTokenEncoder(fakeTokenEncoder{}))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	req, err := http.NewRequest(http.MethodPost, proxyServer.URL+"/v1/chat/completions?stream=true", strings.NewReader(`{"model":"gpt-test"}`))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer sk-test")
	req.Header.Set("Content-Type", "application/json")

	resp, err := proxyServer.Client().Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	var got map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !got["ok"] {
		t.Fatalf("response = %#v, want ok=true", got)
	}
}

func TestHandlerReturnsBadGatewayOnUpstreamFailure(t *testing.T) {
	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: "http://127.0.0.1:1",
	}, withTokenEncoder(fakeTokenEncoder{}))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "TokenGuard: upstream proxy error") {
		t.Fatalf("body = %q, want proxy error", recorder.Body.String())
	}
}

func TestHandlerRoutesConfiguredProviderAndStripsTokenGuardHeaders(t *testing.T) {
	openAICalled := false
	openAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		openAICalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer openAI.Close()

	anthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(tokenGuardProviderHeader); got != "" {
			t.Fatalf("upstream received provider header %q", got)
		}
		if got := r.Header.Get(tokenGuardAPIKeyHeader); got != "" {
			t.Fatalf("upstream received TokenGuard api key %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer anthropic.Close()

	handler, err := NewHandler(Config{
		ListenAddr:        ":0",
		UpstreamURL:       openAI.URL,
		DefaultProvider:   providerOpenAI,
		ProviderRoutes:    map[string]string{providerOpenAI: openAI.URL, providerAnthropic: anthropic.URL},
		GuardEnabled:      false,
		TokenizerModel:    "gpt-4",
		MaxRequestBytes:   1024,
		ReadHeaderTimeout: time.Second,
		ShutdownTimeout:   time.Second,
	}, withTokenEncoder(fakeTokenEncoder{}))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"claude-test"}`))
	req.Header.Set(tokenGuardProviderHeader, providerAnthropic)
	req.Header.Set(tokenGuardAPIKeyHeader, "tg_test")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if openAICalled {
		t.Fatal("default OpenAI upstream was called for Anthropic request")
	}
}

func TestHandlerCountsSSEStreamWithoutChangingBody(t *testing.T) {
	const first = "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n"
	const second = "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n"
	const done = "data: [DONE]\n\n"

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(first))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = w.Write([]byte(second))
		_, _ = w.Write([]byte(done))
	}))
	defer upstream.Close()

	var mu sync.Mutex
	var events []StreamTokenEvent
	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: upstream.URL,
	}, withTokenEncoder(fakeTokenEncoder{}), WithStreamTokenObserver(func(event StreamTokenEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	resp, err := proxyServer.Client().Get(proxyServer.URL + "/v1/chat/completions")
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if string(body) != first+second+done {
		t.Fatalf("body was mutated: %q", string(body))
	}

	mu.Lock()
	defer mu.Unlock()
	if len(events) != 3 {
		t.Fatalf("events = %d, want two deltas plus done", len(events))
	}
	if events[0].Tokens != len("Hello") {
		t.Fatalf("first token count = %d, want %d", events[0].Tokens, len("Hello"))
	}
	if events[1].TotalTokens != int64(len("Hello world")) {
		t.Fatalf("total tokens = %d, want %d", events[1].TotalTokens, len("Hello world"))
	}
	if !events[2].Done {
		t.Fatal("final token event was not marked done")
	}
}

func TestHandlerBlocksInsufficientBudget(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	store := newFakeBudgetStore(5)
	pricing := mustTestPricing(t)
	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: upstream.URL,
	}, withTokenEncoder(fakeTokenEncoder{}), WithGuard(store, pricing, fakeLoopBreaker{}), WithAsyncLogTimeout(time.Second))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set(tokenGuardAPIKeyHeader, "tg_test")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402", recorder.Code)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("proxy error should not set CORS, got Access-Control-Allow-Origin=%q", got)
	}
	if upstreamCalled {
		t.Fatal("upstream was called after budget block")
	}
	event := waitUsageEvent(t, store.events)
	if event.Status != "blocked_budget" {
		t.Fatalf("usage status = %q, want blocked_budget", event.Status)
	}
}

func TestHandlerBlocksCircuitBreakerTrip(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	store := newFakeBudgetStore(100000)
	pricing := mustTestPricing(t)
	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: upstream.URL,
	}, withTokenEncoder(fakeTokenEncoder{}), WithGuard(store, pricing, fakeLoopBreaker{
		result: cache.CircuitBreakerResult{Count: 3, Threshold: 3, Tripped: true},
	}), WithAsyncLogTimeout(time.Second))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","max_tokens":1,"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set(tokenGuardAPIKeyHeader, "tg_test")
	req.Header.Set(tokenGuardSessionHeader, "session-1")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Infinite agent loop detected") {
		t.Fatalf("body = %q, want loop error", recorder.Body.String())
	}
	if upstreamCalled {
		t.Fatal("upstream was called after loop block")
	}
	event := waitUsageEvent(t, store.events)
	if event.Status != "blocked_loop" {
		t.Fatalf("usage status = %q, want blocked_loop", event.Status)
	}
}

func TestHandlerLogsCompletedUsageAndStripsHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(tokenGuardAPIKeyHeader); got != "" {
			t.Fatalf("upstream received TokenGuard key header %q", got)
		}
		if got := r.Header.Get(tokenGuardSessionHeader); got != "" {
			t.Fatalf("upstream received TokenGuard session header %q", got)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	store := newFakeBudgetStore(100000)
	pricing := mustTestPricing(t)
	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: upstream.URL,
	}, withTokenEncoder(fakeTokenEncoder{}), WithGuard(store, pricing, fakeLoopBreaker{}), WithAsyncLogTimeout(time.Second))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	req, err := http.NewRequest(http.MethodPost, proxyServer.URL+"/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","max_tokens":10,"messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("NewRequest returned error: %v", err)
	}
	req.Header.Set(tokenGuardAPIKeyHeader, "tg_test")
	req.Header.Set(tokenGuardSessionHeader, "session-1")

	resp, err := proxyServer.Client().Do(req)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read response body: %v", err)
	}

	event := waitUsageEvent(t, store.events)
	if event.Status != "completed" {
		t.Fatalf("usage status = %q, want completed", event.Status)
	}
	if event.OutputTokens == 0 {
		t.Fatal("OutputTokens = 0, want streamed token count")
	}
	if event.ActualCostMicroUSD == 0 {
		t.Fatal("ActualCostMicroUSD = 0, want charged usage")
	}
}

func TestHandlerLogsProviderUsageFromJSONResponse(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":12},"choices":[{"message":{"content":"done"}}]}`))
	}))
	defer upstream.Close()

	store := newFakeBudgetStore(100000)
	pricing := mustTestPricing(t)
	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: upstream.URL,
	}, withTokenEncoder(fakeTokenEncoder{}), WithGuard(store, pricing, fakeLoopBreaker{}), WithAsyncLogTimeout(time.Second))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`))
	req.Header.Set(tokenGuardAPIKeyHeader, "tg_test")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	event := waitUsageEvent(t, store.events)
	if event.InputTokens != 5 {
		t.Fatalf("InputTokens = %d, want provider-reported 5", event.InputTokens)
	}
	if event.OutputTokens != 7 {
		t.Fatalf("OutputTokens = %d, want provider-reported 7", event.OutputTokens)
	}
}

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

func TestWriteJSONOmitsCORS(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeJSON(recorder, http.StatusPaymentRequired, map[string]string{"error": "budget"})
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("writeJSON set CORS origin %q", got)
	}
}

func TestWriteManagementJSONSetsCORS(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeManagementJSON(recorder, http.StatusOK, map[string]string{"ok": "true"})
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "X-TokenGuard-Admin-Secret") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want admin secret allowed", got)
	}
}

