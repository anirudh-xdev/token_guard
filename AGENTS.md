# AGENTS.md — TokenGuard

Guidance for coding agents working in this repository.

## What this project is

TokenGuard is a Go reverse proxy that acts as a **financial firewall for LLM APIs**: budget reserve/settle (Turso), agent loop detection (Upstash Redis), multi-provider routing, optional admin mgmt + dashboard.

Human docs: [README.md](README.md), [HOW_TO_USE.md](HOW_TO_USE.md), [docs/INDEX.md](docs/INDEX.md).

## Layout (edit map)

| Path | Owns |
|------|------|
| `cmd/tokenguard/main.go` | Startup, routes, guard wiring |
| `internal/proxy/` | Proxy, guard, providers, mgmt APIs |
| `internal/billing/` | Turso schema, budgets, keys, usage |
| `internal/cache/` | Upstash REST + circuit breaker |
| `internal/models/` | Pricing load + cost estimate |
| `internal/ui/` | Embedded admin dashboard |
| `pricing.json` | Allowed models and micro-USD rates |
| `.env.example` | Env contract |

## Invariants (do not break)

1. **Never guess pricing** — unknown models must be blocked; update `pricing.json`.
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

## Useful commands

```powershell
go test ./...
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
