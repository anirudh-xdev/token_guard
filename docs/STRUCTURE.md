# Structure

Repository layout and where to make changes.

```text
TokenGuard/
├── cmd/tokenguard/main.go       # Entry: env, guard wiring, mux, shutdown
├── internal/
│   ├── proxy/                   # HTTP proxy, guard, providers, mgmt, dashboard APIs
│   ├── billing/                 # Turso store, schema, budgets, keys, usage
│   ├── cache/                   # Upstash Redis client + circuit breaker
│   ├── models/                  # Pricing engine
│   └── models/                  # Pricing engine
├── pricing.json                 # Optional bootstrap prices (live catalog = Turso + sync)
├── web/                         # /portal + /dashboard + /docs + marketing (Next.js)
├── testdata/                    # Static fixtures for offline e2e
├── test/                        # Offline integration suite (package e2e)
├── .env.example                 # Env template
├── HOW_TO_USE.md                # Operator / integrator guide
├── README.md                    # Quick start
├── AGENTS.md                    # Agent memory
├── docs/                        # This documentation set
└── .cursor/rules/               # Cursor project rules
```

## Package map

### `cmd/tokenguard`

| File | Edit when… |
|------|------------|
| `main.go` | Changing startup order, routes, or which features require which deps |

### `internal/proxy`

| File | Edit when… |
|------|------------|
| `proxy.go` | Core reverse-proxy behavior, usage logging after response |
| `guard.go` | Budget/loop preflight, block responses, settlement hooks |
| `config.go` | New env knobs for the proxy |
| `provider.go` | Provider routing / path inference |
| `request_analysis.go` | Body parsing, token estimate, session/hash payload |
| `stream_counter.go` | SSE / streaming response wrapping and token counting |
| `usage_extract.go` | JSON/SSE usage + text extraction (incl. OpenRouter `usage.cost`) |
| `pricing_bootstrap.go` | Boot/periodic OpenRouter sync into Turso + in-memory catalog |
| `pricing_mgmt.go` | Admin list/upsert/delete/sync pricing APIs |
| `mgmt.go` | User provisioning admin API |
| `dashboard.go` | List users / usage admin APIs |

### `internal/billing`

| File | Edit when… |
|------|------------|
| `schema.go` | Tables, indexes, constraints |
| `store.go` | Connection, migrate, open/close |
| `usage.go` | Reserve / settle / release, key lookup |
| `admin.go` | Create user/key, list users/usage |
| `pricing_catalog.go` | Turso `model_prices` CRUD / seed |

### `internal/cache`

| File | Edit when… |
|------|------------|
| `client.go` | Upstash REST client |
| `circuit_breaker.go` | Loop threshold / TTL / key prefix logic |

### `internal/models`

| File | Edit when… |
|------|------------|
| `pricing.go` | Pricing engine, file load, cost estimate |
| `openrouter_sync.go` | Fetch OpenRouter models API + sync interval env helpers |
| `price_units.go` | USD/micro-USD conversions |

### `web/`

| Path | Edit when… |
|------|------------|
| `src/app/dashboard/` | Operator console page + styles |
| `src/components/DashboardApp.tsx` | Console behavior (calls Go `/mgmt/*`) |
| `src/app/(site)/portal/` | Product portal UI |
| `src/components/IntegratorDocs.tsx` | Integrator guide at `/docs` |
| `src/app/(site)/docs/` | Docs page shell |

### Root assets

| File | Edit when… |
|------|------------|
| `pricing.json` | Optional bootstrap / override seed only |
| `.env.example` | Documenting new environment variables |

## Conventions

- Keep business logic in `internal/*`; `cmd` only wires dependencies.
- Prefer env config over CLI flags.
- Add or update unit tests next to the package you change.
- Do not invent model prices in code—use OpenRouter sync / Turso catalog (or bootstrap `pricing.json`).
