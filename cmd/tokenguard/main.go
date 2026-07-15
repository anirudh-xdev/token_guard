package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
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

	var options []proxy.HandlerOption
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

		pricing, err := models.LoadPricingFile(initCtx, models.PricingFileFromEnv())
		if err != nil {
			log.Fatalf("load pricing: %v", err)
		}

		// Seed empty DB catalog from pricing.json, then prefer DB as live source of truth.
		// Soft-fail: file-backed pricing keeps the proxy up if Turso is slow/flaky.
		seedCatalog(initCtx, store, pricing)

		options = append(options, proxy.WithGuard(store, pricing, breaker))
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

// seedCatalog best-effort syncs pricing.json into Turso and reloads the in-memory engine.
// Failures fall back to the already-loaded file catalog so deploys stay healthy.
func seedCatalog(_ context.Context, store *billing.Store, pricing *models.PricingEngine) {
	// Independent of the shared init deadline — Turso HTTP can be slow on cold start.
	seedCtx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	seed := make(map[string]billing.ModelPrice)
	for key, price := range pricing.Snapshot() {
		seed[key] = billing.ModelPrice{
			ModelKey:        key,
			InputCostPer1K:  price.InputCostPer1KMicroUSD,
			OutputCostPer1K: price.OutputCostPer1KMicroUSD,
		}
	}

	inserted, err := store.SeedModelPrices(seedCtx, seed)
	if err != nil {
		log.Printf("warning: seed model prices skipped (using file pricing): %v", err)
	} else if inserted > 0 {
		log.Printf("seeded %d model prices from pricing file into DB", inserted)
	}

	// Fill any new models from pricing.json without overwriting operator edits.
	if missing, err := store.UpsertMissingModelPrices(seedCtx, seed); err != nil {
		log.Printf("warning: merge missing model prices: %v", err)
	} else if missing > 0 {
		log.Printf("added %d missing model prices from pricing file", missing)
	}

	// Optional: pull live OpenRouter rates on boot (real market prices).
	if strings.EqualFold(strings.TrimSpace(os.Getenv("TOKENGUARD_PRICING_SYNC_OPENROUTER")), "true") {
		syncCtx, syncCancel := context.WithTimeout(context.Background(), 45*time.Second)
		fetched, err := models.FetchOpenRouterPrices(syncCtx)
		syncCancel()
		if err != nil {
			log.Printf("warning: openrouter pricing sync on boot failed: %v", err)
		} else {
			n := 0
			for _, row := range fetched {
				mp := billing.ModelPrice{
					ModelKey:        row.ModelKey,
					InputCostPer1K:  row.InputCostPer1KMicroUSD,
					OutputCostPer1K: row.OutputCostPer1KMicroUSD,
				}
				if err := store.UpsertModelPrice(seedCtx, mp); err != nil {
					log.Printf("warning: openrouter upsert %s: %v", row.ModelKey, err)
					break
				}
				_ = pricing.Upsert(row.ModelKey, models.Price{
					InputCostPer1KMicroUSD:  row.InputCostPer1KMicroUSD,
					OutputCostPer1KMicroUSD: row.OutputCostPer1KMicroUSD,
				})
				n++
			}
			log.Printf("synced %d openrouter price rows on boot", n)
		}
	}

	dbPrices, err := store.LoadModelPriceMap(seedCtx)
	if err != nil {
		log.Printf("warning: load model prices from DB failed (using file pricing): %v", err)
		return
	}
	if len(dbPrices) == 0 {
		log.Printf("pricing catalog empty in DB; continuing with file pricing (%d models)", pricing.ModelCount())
		return
	}
	live := make(map[string]models.Price, len(dbPrices))
	for key, p := range dbPrices {
		live[key] = models.Price{
			InputCostPer1KMicroUSD:  p.InputCostPer1K,
			OutputCostPer1KMicroUSD: p.OutputCostPer1K,
		}
	}
	if err := pricing.ReplaceAll(live); err != nil {
		log.Printf("warning: replace pricing from DB failed (using file pricing): %v", err)
		return
	}
	log.Printf("pricing catalog loaded from DB (%d models)", len(live))
}
