# Stack

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
| **Turso** | libSQL (`TURSO_DATABASE_URL`, `TURSO_AUTH_TOKEN`) | Users, API keys, budgets, usage events |
| **Upstash Redis** | REST (`UPSTASH_REDIS_REST_URL`, `UPSTASH_REDIS_REST_TOKEN`) | Agent loop circuit breaker |
| **LLM providers** | HTTPS | Upstream APIs (OpenAI, Anthropic, OpenRouter, Groq, …) |

## Frontend

| Piece | Detail |
|-------|--------|
| `dashboard.html` | Vanilla HTML/JS admin UI; no build step, no SPA framework |

## Data and pricing

| Artifact | Detail |
|----------|--------|
| `pricing.json` | Model prices in micro-USD per 1K input/output tokens |
| SQLite schema (via Turso) | See `internal/billing/schema.go` |

## Tests

Go `testing` package with `*_test.go` beside packages under `internal/proxy`, `internal/billing`, `internal/cache`, and `internal/models`.

```powershell
go test ./...
```

## What is not in the stack (yet)

No Dockerfile, CI workflows, package.json, or Kubernetes manifests in this repo.
