package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
)

type Handler struct {
	target                 *url.URL
	proxy                  *httputil.ReverseProxy
	tokenEncoder           tokenEncoder
	tokenizerModel         string
	tokenObserver          StreamTokenObserver
	budgetStore            BudgetStore
	pricing                *models.PricingEngine
	circuitBreaker         LoopBreaker
	asyncLogTimeout        time.Duration
	maxRequestBytes        int64
	defaultMaxOutputTokens int64
}

type HandlerOption func(*handlerOptions)

type handlerOptions struct {
	tokenEncoder    tokenEncoder
	tokenObserver   StreamTokenObserver
	budgetStore     BudgetStore
	pricing         *models.PricingEngine
	circuitBreaker  LoopBreaker
	asyncLogTimeout time.Duration
}

func WithStreamTokenObserver(observer StreamTokenObserver) HandlerOption {
	return func(options *handlerOptions) {
		options.tokenObserver = observer
	}
}

func withTokenEncoder(encoder tokenEncoder) HandlerOption {
	return func(options *handlerOptions) {
		options.tokenEncoder = encoder
	}
}

func WithGuard(store BudgetStore, pricing *models.PricingEngine, breaker LoopBreaker) HandlerOption {
	return func(options *handlerOptions) {
		options.budgetStore = store
		options.pricing = pricing
		options.circuitBreaker = breaker
	}
}

func WithAsyncLogTimeout(timeout time.Duration) HandlerOption {
	return func(options *handlerOptions) {
		options.asyncLogTimeout = timeout
	}
}

func NewHandler(cfg Config, opts ...HandlerOption) (*Handler, error) {
	cfg = cfg.withDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	target, err := parseUpstreamURL(cfg.UpstreamURL)
	if err != nil {
		return nil, err
	}

	options := handlerOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	if options.asyncLogTimeout == 0 {
		options.asyncLogTimeout = 2 * time.Second
	}
	if options.tokenEncoder == nil {
		encoder, err := newTiktokenEncoder(cfg.TokenizerModel)
		if err != nil {
			return nil, fmt.Errorf("create stream tokenizer: %w", err)
		}
		options.tokenEncoder = encoder
	}

	reverseProxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			pr.Out.Host = target.Host
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("proxy upstream error path=%s error=%v", r.URL.Path, err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": "TokenGuard: upstream proxy error",
			})
		},
		Transport: newTransport(),
	}

	return &Handler{
		target:                 target,
		proxy:                  reverseProxy,
		tokenEncoder:           options.tokenEncoder,
		tokenizerModel:         cfg.TokenizerModel,
		tokenObserver:          options.tokenObserver,
		budgetStore:            options.budgetStore,
		pricing:                options.pricing,
		circuitBreaker:         options.circuitBreaker,
		asyncLogTimeout:        options.asyncLogTimeout,
		maxRequestBytes:        cfg.MaxRequestBytes,
		defaultMaxOutputTokens: cfg.DefaultMaxOutputTokens,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var guard *guardContext
	if h.guardEnabled() {
		var ok bool
		guard, ok = h.preflight(w, r)
		if !ok {
			return
		}
	}

	streamWriter := newSSECountingResponseWriter(w, h.tokenEncoder, h.tokenizerModel, h.tokenObserver)
	h.proxy.ServeHTTP(streamWriter, r)
	streamEvent := streamWriter.Finish()

	if guard != nil {
		h.logCompletedUsageAsync(guard, streamEvent, streamWriter.StatusCode())
	}
}

func (h *Handler) Target() *url.URL {
	if h == nil || h.target == nil {
		return nil
	}

	copied := *h.target
	return &copied
}

func newTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          256,
		MaxIdleConnsPerHost:   64,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
}

type BudgetStore interface {
	LookupAPIKey(ctx context.Context, plaintextKey string) (billing.APIKey, error)
	GetUserBudget(ctx context.Context, userID string) (billing.Budget, error)
	RecordUsage(ctx context.Context, event billing.UsageEvent) error
}

type LoopBreaker interface {
	Check(ctx context.Context, sessionID string, payload []byte) (cache.CircuitBreakerResult, error)
}
