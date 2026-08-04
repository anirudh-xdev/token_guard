package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
)

type Handler struct {
	target                     *url.URL
	defaultProvider            string
	providerRoutes             map[string]providerRoute
	proxy                      *httputil.ReverseProxy
	tokenEncoder               tokenEncoder
	tokenizerModel             string
	tokenObserver              StreamTokenObserver
	budgetStore                BudgetStore
	pricing                    *models.PricingEngine
	circuitBreaker             LoopBreaker
	asyncLogTimeout            time.Duration
	maxRequestBytes            int64
	defaultMaxOutputTokens     int64
	adminSecret                string
	managementEnabled          bool
	accountStore               AccountStore
	portalEnabledFlag          bool
	portalDevLogin             bool
	portalBaseURL              string
	portalDefaultBudgetMicroUSD int64
	portalMaxKeys              int
	portalSessionTTL           time.Duration
	portalSecureCookies        bool
	portalAppURL               string
	portalCORSOrigins          []string
	clerkSecretKey             string
}

type HandlerOption func(*handlerOptions)

type handlerOptions struct {
	tokenEncoder    tokenEncoder
	tokenObserver   StreamTokenObserver
	budgetStore     BudgetStore
	pricing         *models.PricingEngine
	circuitBreaker  LoopBreaker
	asyncLogTimeout time.Duration
	accountStore    AccountStore
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

// WithPortal wires the product account store (Next.js UI calls /portal/api/*).
func WithPortal(store AccountStore) HandlerOption {
	return func(options *handlerOptions) {
		options.accountStore = store
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
	routes, err := buildProviderRoutes(cfg)
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

	initClerk(cfg.ClerkSecretKey)

	reverseProxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			route := providerFromContext(pr.In.Context())
			if route.Upstream == nil {
				route = providerRoute{Name: cfg.DefaultProvider, Upstream: target}
			}
			pr.SetURL(route.Upstream)
			pr.Out.Host = route.Upstream.Host
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

	defaultBudget := cfg.PortalDefaultBudgetMicroUSD
	if defaultBudget <= 0 {
		defaultBudget = 5_000_000 // $5
	}
	maxKeys := cfg.PortalMaxKeys
	if maxKeys <= 0 {
		maxKeys = 5
	}
	sessionTTL := cfg.PortalSessionTTL
	if sessionTTL <= 0 {
		sessionTTL = 30 * 24 * time.Hour
	}

	return &Handler{
		target:                      target,
		defaultProvider:             cfg.DefaultProvider,
		providerRoutes:              routes,
		proxy:                       reverseProxy,
		tokenEncoder:                options.tokenEncoder,
		tokenizerModel:              cfg.TokenizerModel,
		tokenObserver:               options.tokenObserver,
		budgetStore:                 options.budgetStore,
		pricing:                     options.pricing,
		circuitBreaker:              options.circuitBreaker,
		asyncLogTimeout:             options.asyncLogTimeout,
		maxRequestBytes:             cfg.MaxRequestBytes,
		defaultMaxOutputTokens:      cfg.DefaultMaxOutputTokens,
		adminSecret:                 cfg.AdminSecret,
		managementEnabled:           cfg.ManagementEnabled,
		accountStore:                options.accountStore,
		portalEnabledFlag:           cfg.PortalEnabled,
		portalDevLogin:              cfg.PortalDevLogin,
		portalBaseURL:               cfg.PortalBaseURL,
		portalDefaultBudgetMicroUSD: defaultBudget,
		portalMaxKeys:               maxKeys,
		portalSessionTTL:            sessionTTL,
		portalSecureCookies:         cfg.PortalSecureCookies,
		portalAppURL:                cfg.PortalAppURL,
		portalCORSOrigins:           cfg.PortalCORSOrigins,
		clerkSecretKey:              cfg.ClerkSecretKey,
	}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if reservedAppPath(r.URL.Path) {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": "TokenGuard: this path is not the LLM proxy. Enable TOKENGUARD_PORTAL_ENABLED / TOKENGUARD_MGMT_ENABLED and restart, or use /docs.",
			"code":  "not_proxy_path",
		})
		return
	}

	route, ok := selectProviderRoute(r, h.defaultProvider, h.providerRoutes)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "TokenGuard: provider is not configured",
		})
		return
	}
	r = withProviderRoute(r, route)

	var guard *guardContext
	if h.guardEnabled() {
		var ok bool
		guard, ok = h.preflight(w, r)
		if !ok {
			return
		}
	} else {
		stripTokenGuardHeaders(r)
	}

	streamWriter := newSSECountingResponseWriter(w, h.tokenEncoder, h.tokenizerModel, route.Name, h.tokenObserver)
	h.proxy.ServeHTTP(streamWriter, r)
	streamEvent := streamWriter.Finish()

	if guard != nil {
		h.logCompletedUsageAsync(guard, streamEvent, streamWriter.StatusCode())
	}
}

func reservedAppPath(path string) bool {
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		path = "/"
	}
	switch path {
	case "/portal", "/dashboard", "/docs", "/healthz", "/mgmt":
		return true
	}
	return strings.HasPrefix(path, "/portal/") ||
		strings.HasPrefix(path, "/dashboard/") ||
		strings.HasPrefix(path, "/mgmt/") ||
		path == "/v1/tokenguard.json"
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
	ReserveBudget(ctx context.Context, userID string, amountMicroUSD int64) (billing.Budget, bool, error)
	RecordUsage(ctx context.Context, event billing.UsageEvent) error
	SettleReservedUsage(ctx context.Context, event billing.UsageEvent, reservedMicroUSD int64) error
	ReleaseReservation(ctx context.Context, userID string, reservedMicroUSD int64) error
	CreateUser(ctx context.Context, email, name string, limitMicroUSD int64) (string, error)
	CreateAPIKey(ctx context.Context, userID, name string) (string, string, error)
	UpdateUserBudget(ctx context.Context, userID string, limitMicroUSD int64, resetSpent bool) (billing.UserBudgetView, error)
	ListUsers(ctx context.Context) ([]billing.UserBudgetView, error)
	ListRecentUsage(ctx context.Context, limit int) ([]billing.UsageEvent, error)
	ListModelPrices(ctx context.Context) ([]billing.ModelPrice, error)
	UpsertModelPrice(ctx context.Context, price billing.ModelPrice) error
	DeleteModelPrice(ctx context.Context, modelKey string) error
	CountModelPrices(ctx context.Context) (int64, error)
	SeedModelPrices(ctx context.Context, prices map[string]billing.ModelPrice) (int, error)
	UpsertMissingModelPrices(ctx context.Context, prices map[string]billing.ModelPrice) (int, error)
}

type LoopBreaker interface {
	Check(ctx context.Context, sessionID string, payload []byte) (cache.CircuitBreakerResult, error)
}
