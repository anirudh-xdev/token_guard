package billing

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ModelPrice is one priced model key (e.g. "gpt-4o" or "openrouter/openai/gpt-4o-mini").
type ModelPrice struct {
	ModelKey           string `json:"model_key"`
	InputCostPer1K     int64  `json:"input_cost_per_1k"`
	OutputCostPer1K    int64  `json:"output_cost_per_1k"`
}

func (s *Store) ListModelPrices(ctx context.Context) ([]ModelPrice, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("billing store is nil")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT model_key, input_cost_per_1k, output_cost_per_1k
FROM model_prices
ORDER BY model_key ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ModelPrice
	for rows.Next() {
		var p ModelPrice
		if err := rows.Scan(&p.ModelKey, &p.InputCostPer1K, &p.OutputCostPer1K); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Store) CountModelPrices(ctx context.Context) (int64, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("billing store is nil")
	}
	var n int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM model_prices`).Scan(&n)
	return n, err
}

func (s *Store) UpsertModelPrice(ctx context.Context, price ModelPrice) error {
	if s == nil || s.db == nil {
		return errors.New("billing store is nil")
	}
	price.ModelKey = strings.TrimSpace(price.ModelKey)
	if price.ModelKey == "" {
		return errors.New("model_key is required")
	}
	if price.InputCostPer1K < 0 || price.OutputCostPer1K < 0 {
		return errors.New("costs cannot be negative")
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO model_prices (model_key, input_cost_per_1k, output_cost_per_1k, updated_at)
VALUES (?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
ON CONFLICT(model_key) DO UPDATE SET
  input_cost_per_1k = excluded.input_cost_per_1k,
  output_cost_per_1k = excluded.output_cost_per_1k,
  updated_at = excluded.updated_at`,
		price.ModelKey, price.InputCostPer1K, price.OutputCostPer1K)
	if err != nil {
		return fmt.Errorf("upsert model price: %w", err)
	}
	return nil
}

func (s *Store) DeleteModelPrice(ctx context.Context, modelKey string) error {
	if s == nil || s.db == nil {
		return errors.New("billing store is nil")
	}
	modelKey = strings.TrimSpace(modelKey)
	if modelKey == "" {
		return errors.New("model_key is required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM model_prices WHERE model_key = ?`, modelKey)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("model price not found: %s", modelKey)
	}
	return nil
}

// SeedModelPrices inserts prices only when the catalog is empty. Returns rows inserted.
// Uses batched multi-row INSERTs to keep Turso HTTP round-trips low.
func (s *Store) SeedModelPrices(ctx context.Context, prices map[string]ModelPrice) (int, error) {
	if s == nil || s.db == nil {
		return 0, errors.New("billing store is nil")
	}
	count, err := s.CountModelPrices(ctx)
	if err != nil {
		return 0, err
	}
	if count > 0 {
		return 0, nil
	}
	if len(prices) == 0 {
		return 0, nil
	}

	type row struct {
		key string
		in  int64
		out int64
	}
	rows := make([]row, 0, len(prices))
	for key, p := range prices {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		rows = append(rows, row{key: key, in: p.InputCostPer1K, out: p.OutputCostPer1K})
	}
	if len(rows) == 0 {
		return 0, nil
	}

	const batchSize = 40
	inserted := 0
	for i := 0; i < len(rows); i += batchSize {
		end := i + batchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[i:end]
		var b strings.Builder
		b.WriteString(`INSERT INTO model_prices (model_key, input_cost_per_1k, output_cost_per_1k) VALUES `)
		args := make([]any, 0, len(batch)*3)
		for j, r := range batch {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString("(?,?,?)")
			args = append(args, r.key, r.in, r.out)
		}
		if _, err := s.db.ExecContext(ctx, b.String(), args...); err != nil {
			return inserted, fmt.Errorf("seed model prices batch: %w", err)
		}
		inserted += len(batch)
	}
	return inserted, nil
}

// LoadModelPriceMap returns all prices as a map keyed by model_key.
func (s *Store) LoadModelPriceMap(ctx context.Context) (map[string]ModelPrice, error) {
	list, err := s.ListModelPrices(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]ModelPrice, len(list))
	for _, p := range list {
		out[p.ModelKey] = p
	}
	return out, nil
}