package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
	"tokenguard/internal/proxy"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("no .env file loaded: %v", err)
	}

	config, err := proxy.ConfigFromEnv()
	if err != nil {
		log.Fatalf("proxy config: %v", err)
	}
	if config.ManagementEnabled && !config.GuardEnabled {
		log.Fatal("management endpoints require TOKENGUARD_GUARD_ENABLED=true")
	}
	if config.PortalEnabled && !config.GuardEnabled {
		log.Fatal("portal requires TOKENGUARD_GUARD_ENABLED=true")
	}

	var (
		options       []proxy.HandlerOption
		billingStore  *billing.Store
		pricingEngine *models.PricingEngine
	)
	if config.GuardEnabled {
		// Turso/Upstash over the public internet often need more than a few seconds
		// (migrate + seed + redis ping), especially on cold deploy.
		initCtx, initCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer initCancel()

		storeConfig, err := billing.ConfigFromEnv()
		if err != nil {
			log.Fatalf("billing config: %v", err)
		}
		store, err := billing.Open(initCtx, storeConfig)
		if err != nil {
			log.Fatalf("open billing store: %v", err)
		}
		defer store.Close()
		if err := store.Migrate(initCtx); err != nil {
			log.Fatalf("migrate billing schema: %v", err)
		}

		redisConfig, err := cache.ConfigFromEnv()
		if err != nil {
			log.Fatalf("cache config: %v", err)
		}
		redis, err := cache.New(redisConfig)
		if err != nil {
			log.Fatalf("open cache client: %v", err)
		}
		if err := redis.Ping(initCtx); err != nil {
			log.Fatalf("ping upstash redis: %v", err)
		}

		breakerConfig, err := cache.CircuitBreakerConfigFromEnv()
		if err != nil {
			log.Fatalf("circuit breaker config: %v", err)
		}
		breaker, err := cache.NewCircuitBreaker(redis, breakerConfig)
		if err != nil {
			log.Fatalf("open circuit breaker: %v", err)
		}

		pricing, err := models.LoadPricingFileOrEmpty(initCtx, models.PricingFileFromEnv())
		if err != nil {
			log.Fatalf("load pricing: %v", err)
		}

		// Independent of the shared init deadline — Turso HTTP can be slow on cold start.
		seedCtx, seedCancel := context.WithTimeout(context.Background(), 45*time.Second)
		err = proxy.BootstrapPricingCatalog(seedCtx, store, pricing, proxy.BootstrapPricingOptions{
			SyncOpenRouter: models.PricingSyncOpenRouterEnabled(),
		})
		seedCancel()
		if err != nil {
			log.Fatalf("pricing catalog: %v", err)
		}

		billingStore = store
		pricingEngine = pricing
		options = append(options, proxy.WithGuard(store, pricing, breaker))
		if config.PortalEnabled {
			options = append(options, proxy.WithPortal(store))
		}
	} else {
		log.Print("TokenGuard guard disabled; running reverse proxy without budget or loop checks")
	}

	handler, err := proxy.NewHandler(config, options...)
	if err != nil {
		log.Fatalf("proxy handler: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/v1/status", handler.HandlePublicStatus)
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		app := strings.TrimSpace(config.DocsAppURL)
		if app == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":"Docs UI is the Next.js app. Set TOKENGUARD_DOCS_APP_URL (e.g. http://localhost:3000/docs).","code":"docs_ui_not_configured"}`))
			return
		}
		http.Redirect(w, r, app, http.StatusFound)
	})
	mux.HandleFunc("/v1/tokenguard.json", handler.HandleDevInfo)
	if config.PortalEnabled {
		mux.HandleFunc("/portal", handler.HandlePortalPage)
		mux.HandleFunc("/portal/dev/login", handler.WithPortalCORS(handler.HandlePortalDevLogin))
		mux.HandleFunc("/portal/logout", handler.WithPortalCORS(handler.HandlePortalLogout))
		mux.HandleFunc("/portal/api/me", handler.WithPortalCORS(handler.HandlePortalMe))
		mux.HandleFunc("/portal/api/keys", handler.WithPortalCORS(handler.HandlePortalCreateKey))
		mux.HandleFunc("/portal/api/keys/revoke", handler.WithPortalCORS(handler.HandlePortalRevokeKey))
		mux.HandleFunc("/portal/api/teams", handler.WithPortalCORS(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handler.HandlePortalListTeams(w, r)
			case http.MethodPost:
				handler.HandlePortalCreateTeam(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}))
		mux.HandleFunc("/portal/api/teams/budget", handler.WithPortalCORS(handler.HandlePortalUpdateTeamBudget))
		mux.HandleFunc("/portal/api/teams/members", handler.WithPortalCORS(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handler.HandlePortalListTeamMembers(w, r)
			case http.MethodPost:
				handler.HandlePortalAddTeamMember(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}))
		mux.HandleFunc("/portal/api/teams/members/cap", handler.WithPortalCORS(handler.HandlePortalUpdateMemberCap))
		mux.HandleFunc("/portal/api/teams/members/remove", handler.WithPortalCORS(handler.HandlePortalRemoveTeamMember))
		mux.HandleFunc("/portal/api/teams/invites", handler.WithPortalCORS(handler.HandlePortalListPendingInvites))
		mux.HandleFunc("/portal/api/usage", handler.WithPortalCORS(handler.HandlePortalListUsage))
		mux.HandleFunc("/portal/api/overview", handler.WithPortalCORS(handler.HandlePortalOverview))
		log.Print("product portal APIs enabled; UI prefers TOKENGUARD_PORTAL_APP_URL")
	} else {
		mux.HandleFunc("/portal", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Portal is disabled. Set TOKENGUARD_PORTAL_ENABLED=true and restart.","code":"portal_disabled"}`))
		})
	}
	if config.ManagementEnabled {
		mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			app := strings.TrimSpace(config.DashboardAppURL)
			if app == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"Operator UI is the Next.js app. Set TOKENGUARD_DASHBOARD_APP_URL (e.g. http://localhost:3000/dashboard).","code":"dashboard_ui_not_configured"}`))
				return
			}
			http.Redirect(w, r, app, http.StatusFound)
		})
		mux.HandleFunc("/mgmt/provision", handler.HandleProvision)
		mux.HandleFunc("/mgmt/budget", handler.HandleUpdateBudget)
		mux.HandleFunc("/mgmt/users", handler.HandleListUsers)
		mux.HandleFunc("/mgmt/usage", handler.HandleListUsage)
		mux.HandleFunc("/mgmt/pricing", handler.HandleListPricing)
		mux.HandleFunc("/mgmt/pricing/upsert", handler.HandleUpsertPricing)
		mux.HandleFunc("/mgmt/pricing/delete", handler.HandleDeletePricing)
		mux.HandleFunc("/mgmt/pricing/sync/openrouter", handler.HandleSyncOpenRouterPricing)
		log.Printf("management APIs enabled; dashboard UI -> %s", config.DashboardAppURL)
	}
	mux.Handle("/", handler)

	server := &http.Server{
		Addr:              config.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if billingStore != nil {
		// Keep Turso HTTP connections warm — cold reconnects often time out from India/APAC.
		go keepTursoWarm(ctx, billingStore)
	}

	if config.GuardEnabled && models.PricingSyncOpenRouterEnabled() {
		if interval := models.PricingSyncInterval(); interval > 0 {
			log.Printf("openrouter pricing sync interval %s", interval)
			go proxy.RunPricingSyncLoop(ctx, billingStore, pricingEngine, interval)
		}
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("TokenGuard proxy listening on %s -> %s", config.ListenAddr, config.UpstreamURL)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Fatalf("shutdown proxy: %v", err)
		}
		log.Print("TokenGuard proxy stopped")
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve proxy: %v", err)
		}
	}
}

func keepTursoWarm(ctx context.Context, store *billing.Store) {
	if store == nil || store.DB() == nil {
		return
	}
	ticker := time.NewTicker(90 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := store.DB().PingContext(pingCtx)
			cancel()
			if err != nil {
				log.Printf("turso keepalive ping failed: %v", err)
			}
		}
	}
}
