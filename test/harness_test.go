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
	"strings"
	"sync"
	"testing"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
	"tokenguard/internal/proxy"
)

const adminSecret = "tokenguard-e2e-admin-secret"

type harnessOpts struct {
	mgmtEnabled     bool
	guardEnabled    bool
	portalEnabled   bool
	portalDevLogin  bool
	maxRequestBytes int64
	budgetLimit     int64
	breaker         proxy.LoopBreaker
	upstream        http.Handler
	openaiURL       string
	anthropicURL    string
	openrouterURL   string
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
		TokenizerModel:              "gpt-4",
		GuardEnabled:                opts.guardEnabled,
		ManagementEnabled:           opts.mgmtEnabled,
		PortalEnabled:               opts.portalEnabled,
		PortalDevLogin:              opts.portalDevLogin,
		PortalBaseURL:               "http://127.0.0.1",
		PortalDefaultBudgetMicroUSD: 5_000_000,
		PortalMaxKeys:               5,
		PortalSessionTTL:            24 * time.Hour,
		PortalSecureCookies:         false,
		PortalAppURL:                "http://localhost:3000/portal",
		PortalCORSOrigins:           []string{"http://localhost:3000"},
		DashboardAppURL:             "http://localhost:3000/dashboard",
		DocsAppURL:                  "http://localhost:3000/docs",
		ClerkSecretKey:              "sk_test_e2e",
		DefaultMaxOutputTokens:      4096,
		MaxRequestBytes:             opts.maxRequestBytes,
		AdminSecret:                 adminSecret,
	}

	var handlerOpts []proxy.HandlerOption
	if opts.guardEnabled {
		handlerOpts = append(handlerOpts, proxy.WithGuard(store, pricing, breaker), proxy.WithAsyncLogTimeout(time.Second))
	}
	if opts.portalEnabled {
		if !opts.guardEnabled {
			t.Fatal("portal tests require guardEnabled")
		}
		handlerOpts = append(handlerOpts, proxy.WithPortal(store))
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
	mux.HandleFunc("/v1/status", handler.HandlePublicStatus)
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, cfg.DocsAppURL, http.StatusFound)
	})
	mux.HandleFunc("/v1/tokenguard.json", handler.HandleDevInfo)
	if opts.portalEnabled {
		mux.HandleFunc("/portal", handler.HandlePortalPage)
		mux.HandleFunc("/portal/dev/login", handler.HandlePortalDevLogin)
		mux.HandleFunc("/portal/logout", handler.HandlePortalLogout)
		mux.HandleFunc("/portal/api/me", handler.HandlePortalMe)
		mux.HandleFunc("/portal/api/keys", handler.HandlePortalCreateKey)
		mux.HandleFunc("/portal/api/keys/revoke", handler.HandlePortalRevokeKey)
		mux.HandleFunc("/portal/api/teams", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handler.HandlePortalListTeams(w, r)
			case http.MethodPost:
				handler.HandlePortalCreateTeam(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		mux.HandleFunc("/portal/api/teams/budget", handler.HandlePortalUpdateTeamBudget)
		mux.HandleFunc("/portal/api/teams/members", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handler.HandlePortalListTeamMembers(w, r)
			case http.MethodPost:
				handler.HandlePortalAddTeamMember(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		mux.HandleFunc("/portal/api/teams/members/cap", handler.HandlePortalUpdateMemberCap)
		mux.HandleFunc("/portal/api/teams/members/remove", handler.HandlePortalRemoveTeamMember)
	}
	if opts.mgmtEnabled {
		mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, cfg.DashboardAppURL, http.StatusFound)
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
	mu         sync.Mutex
	users      map[string]billing.UserBudgetView
	keys       map[string]billing.APIKey // plaintext -> key
	keyMeta    []memKeyMeta
	budgets    map[string]billing.Budget
	prices     map[string]billing.ModelPrice
	usage      []billing.UsageEvent
	events     chan billing.UsageEvent
	oauth      map[string]memOAuth
	sessions   map[string]memSession
	teams      map[string]memTeam
	members    map[string]memTeamMember // teamID|userID
	failLookup bool
	failBudget bool
	seq        int
}

type memTeam struct {
	id, name, owner string
	limit, spent, reserved int64
}

type memTeamMember struct {
	teamID, userID, role, status string
	cap, spent, reserved         int64
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
	prefix := plain
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}
	s.keys[plain] = billing.APIKey{ID: id, UserID: userID, KeyPrefix: prefix}
	s.keyMeta = append(s.keyMeta, memKeyMeta{
		id: id, userID: userID, name: name, prefix: prefix, status: "active",
		createdAt: time.Now().UTC().Format(time.RFC3339), plaintext: plain,
	})
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

// --- portal / account store ---

type memOAuth struct {
	userID string
	email  string
}

type memSession struct {
	id        string
	userID    string
	expiresAt time.Time
	revoked   bool
}

type memKeyMeta struct {
	id        string
	userID    string
	name      string
	prefix    string
	status    string
	createdAt string
	plaintext string
}

func (s *memoryStore) EnsureOAuthUser(ctx context.Context, provider, subject, email, name string, defaultLimitMicroUSD int64) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.oauth == nil {
		s.oauth = map[string]memOAuth{}
	}
	key := provider + ":" + subject
	if existing, ok := s.oauth[key]; ok {
		return existing.userID, false, nil
	}
	email = strings.ToLower(strings.TrimSpace(email))
	for id, u := range s.users {
		if strings.EqualFold(u.Email, email) {
			s.oauth[key] = memOAuth{userID: id, email: email}
			return id, false, nil
		}
	}
	if defaultLimitMicroUSD <= 0 {
		defaultLimitMicroUSD = 1_000_000
	}
	id := s.nextID("user")
	s.users[id] = billing.UserBudgetView{
		UserID: id, Email: email, Name: name, LimitMicroUSD: defaultLimitMicroUSD,
	}
	s.budgets[id] = billing.Budget{UserID: id, LimitMicroUSD: defaultLimitMicroUSD}
	s.oauth[key] = memOAuth{userID: id, email: email}
	return id, true, nil
}

func (s *memoryStore) CreateAuthSession(ctx context.Context, userID string, ttl time.Duration) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sessions == nil {
		s.sessions = map[string]memSession{}
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	plain := "tgs_" + s.nextID("tok")
	s.sessions[plain] = memSession{
		id: s.nextID("sess"), userID: userID, expiresAt: time.Now().UTC().Add(ttl),
	}
	return plain, nil
}

func (s *memoryStore) LookupAuthSession(ctx context.Context, plaintext string) (billing.AuthSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[plaintext]
	if !ok || sess.revoked {
		return billing.AuthSession{}, billing.ErrSessionNotFound
	}
	if time.Now().UTC().After(sess.expiresAt) {
		return billing.AuthSession{}, billing.ErrSessionExpired
	}
	return billing.AuthSession{ID: sess.id, UserID: sess.userID}, nil
}

func (s *memoryStore) RevokeAuthSession(ctx context.Context, plaintext string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[plaintext]; ok {
		sess.revoked = true
		s.sessions[plaintext] = sess
	}
	return nil
}

func (s *memoryStore) GetAccountView(ctx context.Context, userID string) (billing.AccountView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[userID]
	if !ok {
		return billing.AccountView{}, billing.ErrBudgetNotFound
	}
	b := s.budgets[userID]
	available := b.AvailableMicroUSD()
	view := billing.AccountView{
		UserID:            u.UserID,
		Email:             u.Email,
		Name:              u.Name,
		LimitMicroUSD:     b.LimitMicroUSD,
		SpentMicroUSD:     b.SpentMicroUSD,
		ReservedMicroUSD:  b.ReservedMicroUSD,
		AvailableMicroUSD: available,
		BudgetUSD:         float64(b.LimitMicroUSD) / 1_000_000,
		SpentUSD:          float64(b.SpentMicroUSD) / 1_000_000,
		AvailableUSD:      float64(available) / 1_000_000,
		Teams:             []billing.Team{},
	}
	for _, k := range s.keyMeta {
		if k.userID != userID {
			continue
		}
		view.Keys = append(view.Keys, billing.APIKeyMeta{
			ID: k.id, Name: k.name, KeyPrefix: k.prefix, Status: k.status, CreatedAt: k.createdAt,
		})
		if k.status == "active" {
			view.ActiveKeyCount++
		}
	}
	for _, t := range s.teamsForUserLocked(userID) {
		view.Teams = append(view.Teams, t)
	}
	return view, nil
}

func (s *memoryStore) CreateTeam(ctx context.Context, ownerUserID, name string, limitMicroUSD int64) (billing.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.teams == nil {
		s.teams = map[string]memTeam{}
		s.members = map[string]memTeamMember{}
	}
	if limitMicroUSD <= 0 {
		limitMicroUSD = 1_000_000
	}
	id := s.nextID("team")
	s.teams[id] = memTeam{id: id, name: name, owner: ownerUserID, limit: limitMicroUSD}
	s.members[id+"|"+ownerUserID] = memTeamMember{teamID: id, userID: ownerUserID, role: "owner", status: "active", cap: limitMicroUSD}
	return billing.Team{
		ID: id, Name: name, OwnerUserID: ownerUserID,
		LimitMicroUSD: limitMicroUSD, BudgetUSD: float64(limitMicroUSD) / 1e6,
		AvailableMicroUSD: limitMicroUSD, AvailableUSD: float64(limitMicroUSD) / 1e6,
		MyRole: "owner", MyCapMicroUSD: limitMicroUSD, MyCapUSD: float64(limitMicroUSD) / 1e6,
	}, nil
}

func (s *memoryStore) ListTeamsForUser(ctx context.Context, userID string) ([]billing.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.teamsForUserLocked(userID), nil
}

func (s *memoryStore) teamsForUserLocked(userID string) []billing.Team {
	out := []billing.Team{}
	for _, m := range s.members {
		if m.userID != userID || m.status != "active" {
			continue
		}
		t := s.teams[m.teamID]
		avail := t.limit - t.spent - t.reserved
		if avail < 0 {
			avail = 0
		}
		out = append(out, billing.Team{
			ID: t.id, Name: t.name, OwnerUserID: t.owner,
			LimitMicroUSD: t.limit, SpentMicroUSD: t.spent, ReservedMicroUSD: t.reserved,
			AvailableMicroUSD: avail,
			BudgetUSD: float64(t.limit) / 1e6, SpentUSD: float64(t.spent) / 1e6, AvailableUSD: float64(avail) / 1e6,
			MyRole: m.role, MyCapMicroUSD: m.cap, MySpentMicroUSD: m.spent,
			MyCapUSD: float64(m.cap) / 1e6, MySpentUSD: float64(m.spent) / 1e6,
		})
	}
	return out
}

func (s *memoryStore) GetTeamForUser(ctx context.Context, teamID, userID string) (billing.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.teamsForUserLocked(userID) {
		if t.ID == teamID {
			return t, nil
		}
	}
	return billing.Team{}, billing.ErrTeamNotFound
}

func (s *memoryStore) UpdateTeamBudget(ctx context.Context, ownerUserID, teamID string, limitMicroUSD int64) (billing.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.teams[teamID]
	if !ok || t.owner != ownerUserID {
		return billing.Team{}, billing.ErrNotTeamOwner
	}
	t.limit = limitMicroUSD
	s.teams[teamID] = t
	m := s.members[teamID+"|"+ownerUserID]
	m.cap = limitMicroUSD
	s.members[teamID+"|"+ownerUserID] = m
	avail := t.limit - t.spent - t.reserved
	if avail < 0 {
		avail = 0
	}
	return billing.Team{
		ID: t.id, Name: t.name, OwnerUserID: t.owner,
		LimitMicroUSD: t.limit, SpentMicroUSD: t.spent, ReservedMicroUSD: t.reserved,
		AvailableMicroUSD: avail,
		BudgetUSD: float64(t.limit) / 1e6, SpentUSD: float64(t.spent) / 1e6, AvailableUSD: float64(avail) / 1e6,
		MyRole: "owner", MyCapMicroUSD: m.cap, MyCapUSD: float64(m.cap) / 1e6,
	}, nil
}

func (s *memoryStore) AddTeamMemberByEmail(ctx context.Context, ownerUserID, teamID, email string, capMicroUSD int64) (billing.TeamMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.teams[teamID]
	if !ok || t.owner != ownerUserID {
		return billing.TeamMember{}, billing.ErrNotTeamOwner
	}
	var memberID, name string
	for id, u := range s.users {
		if strings.EqualFold(u.Email, email) {
			memberID, name = id, u.Name
			break
		}
	}
	if memberID == "" {
		return billing.TeamMember{}, fmt.Errorf("no TokenGuard account for email %s", email)
	}
	key := teamID + "|" + memberID
	if m, ok := s.members[key]; ok && m.status == "active" {
		return billing.TeamMember{}, billing.ErrTeamMemberExists
	}
	s.members[key] = memTeamMember{teamID: teamID, userID: memberID, role: "member", status: "active", cap: capMicroUSD}
	return billing.TeamMember{
		UserID: memberID, Email: email, Name: name, Role: "member",
		CapMicroUSD: capMicroUSD, CapUSD: float64(capMicroUSD) / 1e6,
		AvailableMicroUSD: capMicroUSD, AvailableUSD: float64(capMicroUSD) / 1e6,
	}, nil
}

func (s *memoryStore) UpdateTeamMemberCap(ctx context.Context, ownerUserID, teamID, memberUserID string, capMicroUSD int64) (billing.TeamMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.teams[teamID]
	if !ok || t.owner != ownerUserID {
		return billing.TeamMember{}, billing.ErrNotTeamOwner
	}
	key := teamID + "|" + memberUserID
	m, ok := s.members[key]
	if !ok || m.status != "active" {
		return billing.TeamMember{}, billing.ErrTeamMemberNotFound
	}
	m.cap = capMicroUSD
	s.members[key] = m
	u := s.users[memberUserID]
	return billing.TeamMember{
		UserID: memberUserID, Email: u.Email, Name: u.Name, Role: m.role,
		CapMicroUSD: capMicroUSD, CapUSD: float64(capMicroUSD) / 1e6,
		SpentMicroUSD: m.spent, SpentUSD: float64(m.spent) / 1e6,
	}, nil
}

func (s *memoryStore) RemoveTeamMember(ctx context.Context, ownerUserID, teamID, memberUserID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.teams[teamID]
	if !ok || t.owner != ownerUserID {
		return billing.ErrNotTeamOwner
	}
	if memberUserID == ownerUserID {
		return fmt.Errorf("cannot remove team owner")
	}
	key := teamID + "|" + memberUserID
	m, ok := s.members[key]
	if !ok {
		return billing.ErrTeamMemberNotFound
	}
	m.status = "removed"
	s.members[key] = m
	return nil
}

func (s *memoryStore) ListTeamMembers(ctx context.Context, requesterUserID, teamID string) ([]billing.TeamMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m, ok := s.members[teamID+"|"+requesterUserID]; !ok || m.status != "active" {
		return nil, billing.ErrTeamNotFound
	}
	var out []billing.TeamMember
	for _, m := range s.members {
		if m.teamID != teamID || m.status != "active" {
			continue
		}
		u := s.users[m.userID]
		avail := m.cap - m.spent - m.reserved
		if avail < 0 {
			avail = 0
		}
		out = append(out, billing.TeamMember{
			UserID: m.userID, Email: u.Email, Name: u.Name, Role: m.role,
			CapMicroUSD: m.cap, SpentMicroUSD: m.spent, ReservedMicroUSD: m.reserved,
			AvailableMicroUSD: avail,
			CapUSD: float64(m.cap) / 1e6, SpentUSD: float64(m.spent) / 1e6, AvailableUSD: float64(avail) / 1e6,
		})
	}
	return out, nil
}

func (s *memoryStore) CountActiveAPIKeys(ctx context.Context, userID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, k := range s.keyMeta {
		if k.userID == userID && k.status == "active" {
			n++
		}
	}
	return n, nil
}

func (s *memoryStore) RevokeAPIKey(ctx context.Context, userID, keyID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, k := range s.keyMeta {
		if k.userID == userID && k.id == keyID && k.status == "active" {
			s.keyMeta[i].status = "revoked"
			if plain := k.plaintext; plain != "" {
				delete(s.keys, plain)
			}
			return nil
		}
	}
	return fmt.Errorf("api key not found or already revoked")
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
	_ proxy.BudgetStore  = (*memoryStore)(nil)
	_ proxy.AccountStore = (*memoryStore)(nil)
	_ proxy.LoopBreaker  = fixedBreaker{}
	_ proxy.LoopBreaker  = (*countingBreaker)(nil)
)
