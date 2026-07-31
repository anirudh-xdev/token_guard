# Why This Stack

Short rationale for each technology in TokenGuard. For the inventory (versions, packages), see [docs/STACK.md](docs/STACK.md). For deeper design trade-offs, see [docs/DESIGN.md](docs/DESIGN.md).

## Core proxy: Go

**Why Go:** TokenGuard is a reverse proxy on the hot path of every LLM call. Go gives a single static binary, strong `net/http` + `httputil.ReverseProxy`, cheap concurrency, and low memory — good for a budget firewall that must stay fast and fail closed.

**Why not Node/Python for the proxy:** Fine for apps; less ideal for a long-lived, multi-tenant proxy where we want predictable latency and one deployable binary with no runtime install.

## Durable money store: Turso (libSQL / SQLite)

**Why Turso:** Budgets, API key hashes, usage events, and model prices need a real ledger — durable rows, migrations, and integer micro-USD math. Turso is hosted libSQL (SQLite family): serverless-friendly, simple schema, works well from a single Go process on Render.

**Why not Postgres first:** Early product does not need multi-writer Postgres complexity. SQLite-shaped storage is enough for per-user budgets and usage; we can migrate later if scale demands it.

**Why not Redis for money:** Redis is great for counters with TTL, not for a financial source of truth.

## Loop detection: Upstash Redis (REST)

**Why Upstash:** Agent loop detection needs short-lived keys (hash → count → expire). Redis fits that; Upstash’s **HTTP REST** API avoids managing a Redis TCP connection from ephemeral hosts and works cleanly behind firewalls.

**Why not Turso for loops:** Writing every identical prompt hit to SQL would be slower and noisier than a TTL counter. Ledger stays in Turso; ephemeral circuit state stays in Redis.

**Fail closed:** If Redis is required and unavailable, guarded mode returns `503` rather than allowing loops through.

## Token estimates: tiktoken-go

**Why tiktoken:** Preflight cost needs an input-token estimate before calling the provider. OpenAI’s tiktoken encodings (via `tiktoken-go`) are the practical standard for GPT-family counting.

**Caveat:** Estimates are approximate across providers; settlement uses real usage (and OpenRouter `usage.cost` when present).

## Config: environment + godotenv

**Why env-only:** One binary, secrets via host env (Render, Docker, local `.env`). No flag sprawl.

**Why godotenv:** Loads `.env` for local/dev; production can set real environment variables and skip the file.

## Pricing: OpenRouter sync + Turso `model_prices` (+ optional `pricing.json`)

**Why OpenRouter models API:** Hundreds of models change price often. Syncing published rates beats hand-editing a file for every model.

**Why Turso catalog as live source:** Operators can override, upsert, and audit prices; the proxy never guesses missing models.

**Why `pricing.json` still exists:** Offline smoke tests, bootstrap before sync, rare sticky overrides — not the day-to-day catalog.

## Admin UI: embedded HTML (`go:embed`)

**Why vanilla HTML/JS in the binary:** Operators need provision / budget / pricing without a separate frontend build for the control plane. One file embeds into the Go binary and works on Render even when cwd ≠ repo root.

## Public docs & discovery: same Go process

**Why `/docs` and `/v1/tokenguard.json` in-process:** Integrators get a how-to and machine-readable providers/models from the same host as the proxy — no extra service for the MVP.

## Marketing site: Next.js (`web/`)

**Why separate Next.js app:** Product marketing (landing, architecture explainer) wants a modern site and its own deploy. It is **not** the budget firewall; the proxy stays Go. Deploy `web/` independently of the TokenGuard binary.

## Deploy: Render (+ Docker)

**Why Render:** Fits a single web service binding `0.0.0.0:$PORT`, env secrets for Turso/Upstash, and a Dockerfile for reproducible builds. Ephemeral disk is fine — state lives in Turso/Redis, not local files.

## Tests: Go `testing` + httptest mocks

**Why no cloud in CI unit/e2e suite:** Offline fixtures in `testdata/` prove guard, routing, and mgmt behavior without Turso/Upstash credentials. Keep money and loop logic testable on every `go test`.

## LLM providers (OpenAI, Anthropic, OpenRouter, Groq, …)

**Why multi-provider routing:** Apps already use many APIs. TokenGuard is a transparent proxy — keep provider auth headers, strip only `X-TokenGuard-*`, settle into one ledger.

---

## Quick map

| Technology | Job in TokenGuard |
|------------|-------------------|
| **Go** | Proxy, guard, mgmt APIs, embed UI |
| **Turso** | Users, keys, budgets, usage, prices |
| **Upstash Redis** | Agent loop circuit breaker |
| **tiktoken** | Preflight token / cost estimate |
| **godotenv** | Local `.env` loading |
| **OpenRouter** | Live model price sync (+ optional billed cost) |
| **pricing.json** | Optional bootstrap / offline seed |
| **Embedded HTML** | Operator dashboard |
| **Next.js (`web/`)** | Marketing site only |
| **Render / Docker** | Host the proxy binary |

```text
App ──► TokenGuard (Go)
           │
           ├── Turso ──────── durable money + prices
           ├── Upstash ────── short-lived loop counters
           ├── tiktoken ───── estimate before spend
           └── Upstream LLM ─ after checks pass
```
