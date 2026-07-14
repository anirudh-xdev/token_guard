# Structure

Repository layout and where to make changes.

```text
TokenGuard/
├── cmd/tokenguard/main.go       # Entry: env, guard wiring, mux, shutdown
├── internal/
│   ├── proxy/                   # HTTP proxy, guard, providers, mgmt, dashboard APIs
│   ├── billing/                 # Turso store, schema, budgets, keys, usage
│   ├── cache/                   # Upstash Redis client + circuit breaker
│   └── models/                  # Pricing engine
├── dashboard.html               # Admin UI
├── pricing.json                 # Allowed models + costs
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
| `stream_counter.go` | SSE / streaming token accounting |
| `mgmt.go` | User provisioning admin API |
| `dashboard.go` | List users / usage admin APIs |

### `internal/billing`

| File | Edit when… |
|------|------------|
| `schema.go` | Tables, indexes, constraints |
| `store.go` | Connection, migrate, open/close |
| `usage.go` | Reserve / settle / release, key lookup |
| `admin.go` | Create user/key, list users/usage |

### `internal/cache`

| File | Edit when… |
|------|------------|
| `client.go` | Upstash REST client |
| `circuit_breaker.go` | Loop threshold / TTL / key prefix logic |

### `internal/models`

| File | Edit when… |
|------|------------|
| `pricing.go` | Pricing file format or cost math |

### Root assets

| File | Edit when… |
|------|------------|
| `pricing.json` | Adding or updating model prices |
| `dashboard.html` | Admin UI behavior or layout |
| `.env.example` | Documenting new environment variables |

## Conventions

- Keep business logic in `internal/*`; `cmd` only wires dependencies.
- Prefer env config over CLI flags.
- Add or update unit tests next to the package you change.
- Do not invent model prices in code—update `pricing.json`.
