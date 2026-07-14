package billing

func schemaStatements() []string {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  name TEXT,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'suspended', 'deleted')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name TEXT NOT NULL DEFAULT 'default',
  key_prefix TEXT NOT NULL UNIQUE,
  key_hash TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK (status IN ('active', 'revoked')),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
  last_used_at TEXT,
  expires_at TEXT,
  revoked_at TEXT
)`,
		`CREATE TABLE IF NOT EXISTS user_budgets (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  currency TEXT NOT NULL DEFAULT 'USD',
  period TEXT NOT NULL DEFAULT 'monthly'
    CHECK (period IN ('daily', 'monthly', 'lifetime')),
  period_start_at TEXT NOT NULL,
  period_end_at TEXT,
  limit_microusd INTEGER NOT NULL CHECK (limit_microusd >= 0),
  spent_microusd INTEGER NOT NULL DEFAULT 0 CHECK (spent_microusd >= 0),
  reserved_microusd INTEGER NOT NULL DEFAULT 0 CHECK (reserved_microusd >= 0),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
)`,
		`CREATE TABLE IF NOT EXISTS usage_events (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  api_key_id TEXT REFERENCES api_keys(id) ON DELETE SET NULL,
  provider TEXT NOT NULL,
  model TEXT NOT NULL,
  session_id TEXT,
  request_id TEXT,
  input_tokens INTEGER NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
  output_tokens INTEGER NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
  estimated_cost_microusd INTEGER NOT NULL DEFAULT 0 CHECK (estimated_cost_microusd >= 0),
  actual_cost_microusd INTEGER NOT NULL DEFAULT 0 CHECK (actual_cost_microusd >= 0),
  status TEXT NOT NULL CHECK (
    status IN ('allowed', 'blocked_budget', 'blocked_loop', 'provider_error', 'completed')
  ),
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
)`,
		`CREATE TABLE IF NOT EXISTS model_prices (
  model_key TEXT PRIMARY KEY,
  input_cost_per_1k INTEGER NOT NULL CHECK (input_cost_per_1k >= 0),
  output_cost_per_1k INTEGER NOT NULL CHECK (output_cost_per_1k >= 0),
  updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_hash_status
  ON api_keys(key_hash, status)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_user_created
  ON usage_events(user_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_events_session_created
  ON usage_events(session_id, created_at)`,
	}

	out := make([]string, len(statements))
	copy(out, statements)
	return out
}
