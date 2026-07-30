package proxy

import (
	"context"
	"fmt"
	"log"
	"time"

	"tokenguard/internal/billing"
	"tokenguard/internal/models"
)

// BootstrapPricingOptions controls catalog seed / OpenRouter sync at boot.
type BootstrapPricingOptions struct {
	SyncOpenRouter bool
}

// BootstrapPricingCatalog seeds missing rows from the in-memory snapshot (usually pricing.json),
// optionally imports OpenRouter rates, then reloads Turso into the PricingEngine.
// Returns an error if the catalog is still empty afterward (fail closed).
func BootstrapPricingCatalog(ctx context.Context, store BudgetStore, pricing *models.PricingEngine, opts BootstrapPricingOptions) error {
	if store == nil || pricing == nil {
		return fmt.Errorf("pricing store and engine are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	seed := make(map[string]billing.ModelPrice)
	for key, price := range pricing.Snapshot() {
		seed[key] = billing.ModelPrice{
			ModelKey:        key,
			InputCostPer1K:  price.InputCostPer1KMicroUSD,
			OutputCostPer1K: price.OutputCostPer1KMicroUSD,
		}
	}

	inserted, err := store.SeedModelPrices(ctx, seed)
	if err != nil {
		log.Printf("warning: seed model prices skipped (using in-memory pricing): %v", err)
	} else if inserted > 0 {
		log.Printf("seeded %d model prices from pricing file into DB", inserted)
	}

	if missing, err := store.UpsertMissingModelPrices(ctx, seed); err != nil {
		log.Printf("warning: merge missing model prices: %v", err)
	} else if missing > 0 {
		log.Printf("added %d missing model prices from pricing file", missing)
	}

	if opts.SyncOpenRouter {
		n, err := SyncOpenRouterIntoStore(ctx, store, pricing)
		if err != nil {
			log.Printf("warning: openrouter pricing sync on boot failed: %v", err)
		} else {
			log.Printf("synced %d openrouter price rows on boot", n)
		}
	}

	if err := ReloadPricingFromStore(ctx, store, pricing); err != nil {
		if pricing.ModelCount() == 0 {
			return fmt.Errorf("%w; provide pricing.json and/or enable OpenRouter sync", err)
		}
		log.Printf("warning: %v (continuing with in-memory pricing, %d models)", err, pricing.ModelCount())
		return nil
	}

	if pricing.ModelCount() == 0 {
		return fmt.Errorf("no pricing catalog: provide pricing.json and/or enable OpenRouter sync (TOKENGUARD_PRICING_SYNC_OPENROUTER)")
	}
	log.Printf("pricing catalog ready (%d models)", pricing.ModelCount())
	return nil
}

// SyncOpenRouterIntoStore imports live OpenRouter model prices into the store and in-memory engine.
func SyncOpenRouterIntoStore(ctx context.Context, store BudgetStore, pricing *models.PricingEngine) (int, error) {
	if store == nil || pricing == nil {
		return 0, fmt.Errorf("pricing store and engine are required")
	}
	fetched, err := models.FetchOpenRouterPrices(ctx)
	if err != nil {
		return 0, err
	}
	imported := 0
	for _, row := range fetched {
		mp := billing.ModelPrice{
			ModelKey:        row.ModelKey,
			InputCostPer1K:  row.InputCostPer1KMicroUSD,
			OutputCostPer1K: row.OutputCostPer1KMicroUSD,
		}
		if err := store.UpsertModelPrice(ctx, mp); err != nil {
			return imported, err
		}
		_ = pricing.Upsert(row.ModelKey, models.Price{
			InputCostPer1KMicroUSD:  row.InputCostPer1KMicroUSD,
			OutputCostPer1KMicroUSD: row.OutputCostPer1KMicroUSD,
		})
		imported++
	}
	return imported, nil
}

// ReloadPricingFromStore replaces the in-memory catalog from store rows.
func ReloadPricingFromStore(ctx context.Context, store BudgetStore, pricing *models.PricingEngine) error {
	if store == nil || pricing == nil {
		return fmt.Errorf("pricing store and engine are required")
	}
	list, err := store.ListModelPrices(ctx)
	if err != nil {
		return fmt.Errorf("load model prices from DB: %w", err)
	}
	if len(list) == 0 {
		return fmt.Errorf("pricing catalog empty in DB")
	}
	live := make(map[string]models.Price, len(list))
	for _, p := range list {
		live[p.ModelKey] = models.Price{
			InputCostPer1KMicroUSD:  p.InputCostPer1K,
			OutputCostPer1KMicroUSD: p.OutputCostPer1K,
		}
	}
	if err := pricing.ReplaceAll(live); err != nil {
		return fmt.Errorf("replace pricing from DB: %w", err)
	}
	return nil
}

// RunPricingSyncLoop periodically refreshes OpenRouter prices until ctx is cancelled.
// Failures are logged; the last-good catalog is kept.
func RunPricingSyncLoop(ctx context.Context, store BudgetStore, pricing *models.PricingEngine, interval time.Duration) {
	if interval <= 0 || store == nil || pricing == nil {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
			n, err := SyncOpenRouterIntoStore(syncCtx, store, pricing)
			if err != nil {
				log.Printf("warning: periodic openrouter pricing sync failed: %v", err)
				cancel()
				continue
			}
			if err := ReloadPricingFromStore(syncCtx, store, pricing); err != nil {
				log.Printf("warning: reload pricing after periodic sync: %v (upserted %d rows into memory)", err, n)
			} else {
				log.Printf("periodic openrouter pricing sync imported %d rows (%d models in catalog)", n, pricing.ModelCount())
			}
			cancel()
		}
	}
}
