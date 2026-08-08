package proxy

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"tokenguard/internal/cache"
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

func TestHandlerLogsProviderUsageFromGzipJSON(t *testing.T) {
	var gzBuf bytes.Buffer
	zw := gzip.NewWriter(&gzBuf)
	_, _ = zw.Write([]byte(`{"usage":{"prompt_tokens":4398,"completion_tokens":4813,"total_tokens":9211},"choices":[{"message":{"content":"ok"}}]}`))
	_ = zw.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a gateway that returns gzip even when the proxy did not ask for it.
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(gzBuf.Bytes())
	}))
	defer upstream.Close()

	store := newFakeBudgetStore(100_000_000)
	pricing := mustTestPricing(t)
	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: upstream.URL,
	}, withTokenEncoder(fakeTokenEncoder{}), WithGuard(store, pricing, fakeLoopBreaker{}), WithAsyncLogTimeout(time.Second))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","max_tokens":20,"messages":[{"role":"user","content":"write 5000 words"}]}`))
	req.Header.Set(tokenGuardAPIKeyHeader, "tg_test")
	req.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-TokenGuard-Usage"); got != "in=4398;out=4813" {
		t.Fatalf("X-TokenGuard-Usage = %q, want in=4398;out=4813", got)
	}
	event := waitUsageEvent(t, store.events)
	if event.InputTokens != 4398 || event.OutputTokens != 4813 {
		t.Fatalf("usage tokens = %d/%d, want 4398/4813", event.InputTokens, event.OutputTokens)
	}
}

func TestHandlerLogsProviderUsageWithoutJSONContentType(t *testing.T) {
	// Reproduces APIMaster-style responses that omit application/json.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"chatcmpl-x","choices":[{"message":{"role":"assistant","content":"long answer"}}],"usage":{"prompt_tokens":4398,"completion_tokens":5572,"total_tokens":9970,"prompt_tokens_details":{"cached_tokens":3840}}}`))
	}))
	defer upstream.Close()

	store := newFakeBudgetStore(100_000_000)
	pricing := mustTestPricing(t)
	handler, err := NewHandler(Config{
		ListenAddr:  ":0",
		UpstreamURL: upstream.URL,
	}, withTokenEncoder(fakeTokenEncoder{}), WithGuard(store, pricing, fakeLoopBreaker{}), WithAsyncLogTimeout(time.Second))
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"gpt-test","max_tokens":20,"messages":[{"role":"user","content":"write 5000 words"}]}`))
	req.Header.Set(tokenGuardAPIKeyHeader, "tg_test")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 body=%s", recorder.Code, recorder.Body.String())
	}
	event := waitUsageEvent(t, store.events)
	if event.InputTokens != 4398 {
		t.Fatalf("InputTokens = %d, want 4398 from provider usage", event.InputTokens)
	}
	if event.OutputTokens != 5572 {
		t.Fatalf("OutputTokens = %d, want 5572 from provider usage", event.OutputTokens)
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

