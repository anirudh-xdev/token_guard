package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"tokenguard/internal/billing"
	"tokenguard/internal/cache"
	"tokenguard/internal/models"
	"tokenguard/internal/proxy"
	"tokenguard/internal/ui"
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
			options = append(options, proxy.WithPortal(store, ui.PortalHTML))
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
	mux.HandleFunc("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(ui.DocsHTML)
	})
	mux.HandleFunc("/v1/tokenguard.json", handler.HandleDevInfo)
	if config.PortalEnabled {
		mux.HandleFunc("/portal", handler.HandlePortalPage)
		mux.HandleFunc("/portal/login/github", handler.HandlePortalGitHubLogin)
		mux.HandleFunc("/portal/callback/github", handler.HandlePortalGitHubCallback)
		mux.HandleFunc("/portal/dev/login", handler.HandlePortalDevLogin)
		mux.HandleFunc("/portal/logout", handler.HandlePortalLogout)
		mux.HandleFunc("/portal/api/me", handler.HandlePortalMe)
		mux.HandleFunc("/portal/api/keys", handler.HandlePortalCreateKey)
		mux.HandleFunc("/portal/api/keys/revoke", handler.HandlePortalRevokeKey)
		log.Print("product portal enabled at /portal")
	} else {
		mux.HandleFunc("/portal", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"Portal is disabled. Set TOKENGUARD_PORTAL_ENABLED=true and restart.","code":"portal_disabled"}`))
		})
	}
	if config.ManagementEnabled {
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

	server := &http.Server{
		Addr:              config.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: config.ReadHeaderTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
