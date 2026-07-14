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

	var options []proxy.HandlerOption
	if config.GuardEnabled {
		initCtx, initCancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	if config.ManagementEnabled {
		mux.HandleFunc("/dashboard", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(ui.DashboardHTML)
		})
		mux.HandleFunc("/mgmt/provision", handler.HandleProvision)
		mux.HandleFunc("/mgmt/users", handler.HandleListUsers)
		mux.HandleFunc("/mgmt/usage", handler.HandleListUsage)
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
