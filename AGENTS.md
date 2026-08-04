# AGENTS.md — TokenGuard

Guidance for coding agents working in this repository.

## What this project is

TokenGuard is a Go reverse proxy that acts as a **financial firewall for LLM APIs**: budget reserve/settle (Turso), agent loop detection (Upstash Redis), multi-provider routing, optional admin mgmt + dashboard.

Human docs: [README.md](README.md), [HOW_TO_USE.md](HOW_TO_USE.md), [docs/INDEX.md](docs/INDEX.md).

## Product surfaces

| Audience | Path | Auth |
|----------|------|------|
| End users (hosted product) | Next.js `web/` `/portal` + Go `/portal/api/*` | Clerk (Next) → Bearer JWT to Go |
| Operators | Next.js `web/` `/dashboard` + Go `/mgmt/*` | `TOKENGUARD_ADMIN_SECRET` |
| Integrators | `/docs`, `/v1/tokenguard.json` | Public |

Do not mix privileges: portal users must never reach `ListUsers` / pricing sync.

## Layout (edit map)

| Path | Owns |
|------|------|
| `cmd/tokenguard/main.go` | Startup, routes, guard wiring |
| `internal/proxy/` | Proxy, guard, providers, mgmt APIs |
| `internal/billing/` | Turso schema, budgets, keys, usage |
| `internal/cache/` | Upstash REST + circuit breaker |
| `internal/models/` | Pricing load + cost estimate |
| `web/` | Product UI (`/portal`), operator console (`/dashboard`), integrator docs (`/docs`), marketing |
| `pricing.json` | Optional bootstrap prices (seed only); live catalog = Turso + OpenRouter sync |
| `testdata/` | Static JSON/SSE fixtures for offline e2e |
| `test/` | Offline integration suite (mocks, no cloud) |
| `.env.example` | Env contract |

## Invariants (do not break)

1. **Never guess pricing** — unknown models must be blocked. Prefer OpenRouter sync / Turso catalog; use `pricing.json` only as bootstrap or rare overrides.
2. **Strip `X-TokenGuard-*` headers** before forwarding upstream.
3. **Management requires guard** — `TOKENGUARD_MGMT_ENABLED` implies `TOKENGUARD_GUARD_ENABLED`.
4. **Money is micro-USD integers** — no floats in the ledger.
5. **API keys hashed at rest** — plaintext `tg_` only at provision time.
6. **Config is env-only** — no new CLI flags unless explicitly requested.
7. **Fail closed** when Redis/Turso are required but unavailable (guarded mode → `503`).

## Conventions

- Keep `cmd` thin; logic lives in `internal/*`.
- Prefer small focused changes; update neighboring `*_test.go`.
- Match existing naming: `ConfigFromEnv`, `WithGuard`, microusd fields.
- Do not commit `.env`, secrets, or binaries (`tokenguard.exe`).
- Commits: short conventional subjects (`fix: …`, `feat: …`); one concern each. See `.cursor/rules/git-commits.mdc`.

## Useful commands

```powershell
go test ./...
go test ./test/... -count=1
go build -o tokenguard.exe ./cmd/tokenguard
```

## Status codes (guard)

- `401` missing/invalid TokenGuard key
- `400` bad request / unknown model pricing
- `402` budget
- `409` loop breaker
- `413` body too large
- `503` store/breaker unavailable

See [docs/API.md](docs/API.md) and [docs/DESIGN.md](docs/DESIGN.md).
