# Stack

Inventory of languages, libraries, and services. For **why** each piece exists, see [WHY_THIS_STACK.md](../WHY_THIS_STACK.md).

## Language and runtime

| Piece | Detail |
|-------|--------|
| Language | Go **1.21** (`go.mod`) |
| Module | `tokenguard` |
| HTTP | Standard library `net/http`, `httputil.ReverseProxy` |
| Config | Env vars + optional `.env` via `github.com/joho/godotenv` |

## Direct dependencies

From `go.mod`:

| Package | Role |
|---------|------|
| `github.com/joho/godotenv` | Load `.env` at startup |
| `github.com/pkoukk/tiktoken-go` | Token estimation for preflight cost |
| `github.com/tursodatabase/libsql-client-go` | Turso / libSQL client |

## External services

| Service | Protocol | Used for |
|---------|----------|----------|
| **Turso** | libSQL (`TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`) | Users, API keys, budgets, usage events, model prices |
| **Upstash Redis** | REST (`UPSTASH_REDIS_REST_URL`, `UPSTASH_REDIS_REST_TOKEN`) | Agent loop circuit breaker |
| **LLM providers** | HTTPS | Upstream APIs (OpenAI, Anthropic, OpenRouter, Groq, …) |

## Frontend

| Piece | Detail |
|-------|--------|
| Frontend | Next.js `web/` — portal, dashboard, integrator docs (not in the Go binary) |

## Data and pricing

| Artifact | Detail |
|----------|--------|
| `pricing.json` | Optional bootstrap; live rates from Turso / OpenRouter sync |
| SQLite schema (via Turso) | See `internal/billing/schema.go` |

## Tests

Go `testing` package:

| Location | Role |
|----------|------|
| `internal/*/…_test.go` | Unit tests beside packages |
| `test/` + `testdata/` | Offline integration suite (static JSON fixtures, httptest mocks, no Turso/Redis) |

```powershell
go test ./...
go test ./test/... -count=1
```

Fixtures live in `testdata/` (chat, pricing, mgmt, OpenRouter sync mocks). Override OpenRouter sync URL in tests via `TOKENGUARD_OPENROUTER_MODELS_URL`.

## Product frontend (separate deploy)

| Piece | Detail |
|-------|--------|
| `web/` | Next.js product UI (`/portal`), operator console (`/dashboard`), marketing |

## What is not in the stack (yet)

No CI workflows or Kubernetes manifests in this repo.
