# TokenGuard: How To Use It

TokenGuard is a financial firewall for AI apps. It sits between your application and LLM providers such as OpenAI, Anthropic Claude, OpenRouter, Groq, Mistral, and other OpenAI-compatible APIs.

For product overview, architecture, and agent guidance, see [docs/INDEX.md](docs/INDEX.md), [README.md](README.md), and [AGENTS.md](AGENTS.md). Full HTTP reference: [docs/API.md](docs/API.md).

Instead of sending requests directly to an AI provider, your app sends them to TokenGuard first. TokenGuard checks budget, estimates cost, detects repeated agent loops, tracks usage, and only forwards safe requests to the provider.

## What Problem It Solves

AI API bills can grow silently and quickly.

Common failure cases:

- A user sends very large prompts repeatedly.
- An autonomous agent gets stuck in a loop.
- A bug retries expensive requests too many times.
- A SaaS app needs per-user AI budgets.
- A team uses many providers and cannot easily track usage in one place.

TokenGuard helps by:

- Blocking requests before money is spent.
- Enforcing per-user budgets (operator-controlled).
- Detecting repeated prompt/tool-call loops.
- Tracking input tokens, output tokens, and cost.
- Supporting multiple providers through one proxy.
- Fail-closing when a model is not in the pricing catalog.

## How The Flow Works

Without TokenGuard:

```text
Your App -> OpenAI/Claude/etc. -> Provider bill grows
```

With TokenGuard:

```text
Your App -> TokenGuard -> Budget/loop/cost checks -> OpenAI/Claude/etc.
```

If a request is safe, TokenGuard forwards it.

If a request is too expensive, over budget, or looks like an agent loop, TokenGuard blocks it with a JSON error (and a machine-readable `code` field).

## Who Uses TokenGuard

TokenGuard is mainly for:

- AI app developers
- SaaS founders
- Teams building autonomous agents
- Agencies building AI products for clients
- Companies using multiple LLM providers

**Operators** (you / your team) hold `TOKENGUARD_ADMIN_SECRET` and provision users, budgets, and prices.

**Apps** use a `tg_...` key plus a real provider API key. End users do not usually interact with TokenGuard directly — and strangers cannot mint keys from the public `/docs` page without the admin secret.

## Local Smoke Test

Use this mode to check that TokenGuard boots without Turso or Upstash credentials.

```powershell
$env:TOKENGUARD_GUARD_ENABLED='false'
$env:TOKENGUARD_LISTEN_ADDR='127.0.0.1:18080'
$env:TIKTOKEN_CACHE_DIR='.tiktoken-cache'

go run ./cmd/tokenguard
# Or: go build -o tokenguard.exe ./cmd/tokenguard; .\tokenguard.exe
```

In another terminal:

```powershell
Invoke-WebRequest http://127.0.0.1:18080/healthz -UseBasicParsing
```

Expected response:

```json
{"status":"ok"}
```

Public pages (no admin secret):

| URL | Purpose |
|-----|---------|
| `/healthz` | Liveness |
| `/docs` | Human integration guide |
| `/portal` | Product sign-in + personal API keys (when portal enabled) |
| `/v1/tokenguard.json` | Machine discovery (providers, bases, priced models — no secrets) |

## Product portal (hosted users)

**Frontend (Next.js `web/`)** handles Clerk sign-in and the portal UI.  
**Backend (Go)** handles `/portal/api/*`, budgets, keys, teams, and the LLM proxy.

```powershell
# Terminal 1 — API
go run ./cmd/tokenguard

# Terminal 2 — UI
cd web
npm run dev
```

Open `http://localhost:3000/portal`.

```env
# Go (.env)
TOKENGUARD_PORTAL_ENABLED=true
TOKENGUARD_PORTAL_APP_URL=http://localhost:3000/portal
TOKENGUARD_PORTAL_CORS_ORIGINS=http://localhost:3000
TOKENGUARD_CLERK_PUBLISHABLE_KEY=pk_...
TOKENGUARD_CLERK_SECRET_KEY=sk_...

# web/.env.local
NEXT_PUBLIC_TOKENGUARD_API_URL=http://127.0.0.1:8080
NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY=pk_...
CLERK_SECRET_KEY=sk_...
```

In Clerk, allow origin `http://localhost:3000`.

User flow: sign in on Next → create key → call `{API}/v1` with `X-TokenGuard-API-Key`. Teams/caps are managed in the portal UI and enforced by the Go API.

Operator `/dashboard` remains on the Go host.

## Real Guarded Mode

Guarded mode enables budgets, usage logging, and loop detection.

Create a `.env` file using [`.env.example`](.env.example) as the template.

Minimum required values:

```env
TOKENGUARD_GUARD_ENABLED=true
TOKENGUARD_LISTEN_ADDR=:8080

TURSO_DATABASE_URL=libsql://your-database.turso.io
TURSO_AUTH_TOKEN=your_turso_token

UPSTASH_REDIS_REST_URL=https://your-redis-instance.upstash.io
UPSTASH_REDIS_REST_TOKEN=your_upstash_token

TOKENGUARD_PRICING_FILE=pricing.json
TOKENGUARD_PRICING_SYNC_OPENROUTER=true
TOKENGUARD_PRICING_SYNC_INTERVAL=6h
TOKENGUARD_DEFAULT_PROVIDER=openai
TOKENGUARD_UPSTREAM_URL=https://api.openai.com

TOKENGUARD_MGMT_ENABLED=true
TOKENGUARD_ADMIN_SECRET=make-this-at-least-16-chars
```

Then run from the repo root:

```powershell
go build -o tokenguard.exe ./cmd/tokenguard
.\tokenguard.exe
```

TokenGuard will:

- Connect to Turso.
- Run database migrations.
- Connect to Upstash Redis.
- Load optional `pricing.json` bootstrap, sync OpenRouter rates into Turso (default when unset), and build the live pricing catalog.
- Start the proxy server (background pricing refresh on `TOKENGUARD_PRICING_SYNC_INTERVAL`).

## Configure Multiple Providers

Use `TOKENGUARD_PROVIDER_ROUTES` to define named providers.

Example:

```env
TOKENGUARD_PROVIDER_ROUTES=openai=https://api.openai.com,anthropic=https://api.anthropic.com,openrouter=https://openrouter.ai/api,groq=https://api.groq.com/openai/v1
```

**OpenRouter base URL:** use `https://openrouter.ai/api` (**not** `.../api/v1`). Clients call `/v1/chat/completions` on TokenGuard; the proxy joins paths. A base ending in `/api/v1` would become `/api/v1/v1/...` and 404. TokenGuard normalizes the known misconfig on load.

Supported style:

```text
provider_name=https://provider-api-base-url
```

Then choose a provider per request with:

```http
X-TokenGuard-Provider: anthropic
```

If no provider is specified, TokenGuard uses `TOKENGUARD_DEFAULT_PROVIDER` (path `/v1/messages` still infers Anthropic).

## Pricing Setup

Every model TokenGuard allows must exist in the **live pricing catalog** (Turso `model_prices` → in-memory engine).

Human-facing rates use **USD per 1M tokens** (`input_usd_per_million` / `output_usd_per_million`). Internally TokenGuard stores micro-USD per 1K for integer budget math.

### Recommended: auto-sync from OpenRouter

Do **not** hand-edit `pricing.json` for every new model. Enable sync (default when unset):

```env
TOKENGUARD_PRICING_SYNC_OPENROUTER=true
TOKENGUARD_PRICING_SYNC_INTERVAL=6h
```

On boot (and on the interval), TokenGuard imports published OpenRouter model rates. You can also use **Pricing → Sync OpenRouter** in the dashboard or:

```http
POST /mgmt/pricing/sync/openrouter
X-TokenGuard-Admin-Secret: your-admin-secret
```

`pricing.json` is optional: bootstrap / offline smoke / rare sticky overrides only.

### Manual overrides

Prefer dashboard upsert or:

```http
POST /mgmt/pricing/upsert
Content-Type: application/json
X-TokenGuard-Admin-Secret: your-admin-secret

{"model_key":"gpt-4o-mini","input_usd_per_million":0.15,"output_usd_per_million":0.6}
```

Optional file bootstrap format:

```json
{
  "gpt-4o-mini": {
    "input_usd_per_million": 0.15,
    "output_usd_per_million": 0.6
  },
  "anthropic/claude-sonnet-4-6": {
    "input_usd_per_million": 3.0,
    "output_usd_per_million": 15.0
  }
}
```

If a model is missing from the catalog, TokenGuard blocks the request with `400` and `"code":"pricing_not_configured"` instead of guessing.

## Provision A User (operator only)

Management endpoints must be enabled:

```env
TOKENGUARD_MGMT_ENABLED=true
TOKENGUARD_ADMIN_SECRET=make-this-at-least-16-chars
```

Create a user, TokenGuard API key, and optional budget (default **$1** if omitted):

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8080/mgmt/provision `
  -Headers @{ "X-TokenGuard-Admin-Secret" = "make-this-at-least-16-chars" } `
  -ContentType "application/json" `
  -Body '{"email":"dev@example.com","name":"Dev User","budget_usd":50}'
```

Response (shape):

```json
{
  "user_id": "user_xxx",
  "api_key": "tg_xxx",
  "api_key_id": "key_xxx",
  "limit_microusd": 50000000,
  "budget_usd": 50,
  "integration": {
    "docs_url": "/docs",
    "dashboard_url": "/dashboard",
    "proxy_url": "/v1/chat/completions"
  }
}
```

Save `api_key` immediately — it is shown once. Apps send it as `X-TokenGuard-API-Key` (or `X-TokenGuard-Key`).

### Extend budget after a 402

```powershell
Invoke-RestMethod `
  -Method Patch `
  -Uri http://127.0.0.1:8080/mgmt/budget `
  -Headers @{ "X-TokenGuard-Admin-Secret" = "make-this-at-least-16-chars" } `
  -ContentType "application/json" `
  -Body '{"user_id":"user_xxx","budget_usd":100,"reset_spent":false}'
```

Set `reset_spent: true` to zero spent when starting a fresh period after raising the limit.

## Call OpenAI Through TokenGuard

Before TokenGuard, your app calls OpenAI directly:

```text
https://api.openai.com/v1/chat/completions
```

After TokenGuard, your app calls:

```text
http://127.0.0.1:8080/v1/chat/completions
```

Example:

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8080/v1/chat/completions `
  -Headers @{
    "Authorization" = "Bearer YOUR_OPENAI_API_KEY"
    "X-TokenGuard-API-Key" = "tg_your_tokenguard_key"
    "X-TokenGuard-Provider" = "openai"
    "X-TokenGuard-Session-ID" = "session-123"
  } `
  -ContentType "application/json" `
  -Body '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}],"max_tokens":100}'
```

TokenGuard forwards `Authorization` to OpenAI but strips its own `X-TokenGuard-*` headers before forwarding.

## Call Claude Through TokenGuard

Configure Anthropic:

```env
TOKENGUARD_PROVIDER_ROUTES=anthropic=https://api.anthropic.com
```

Ensure Anthropic models are in the catalog (OpenRouter sync usually covers them). For a custom rate, upsert via `/mgmt/pricing/upsert` or add a bootstrap row in `pricing.json`.

Request:

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8080/v1/messages `
  -Headers @{
    "x-api-key" = "YOUR_ANTHROPIC_API_KEY"
    "anthropic-version" = "2023-06-01"
    "X-TokenGuard-API-Key" = "tg_your_tokenguard_key"
    "X-TokenGuard-Provider" = "anthropic"
    "X-TokenGuard-Session-ID" = "session-123"
  } `
  -ContentType "application/json" `
  -Body '{"model":"claude-sonnet-4-6","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}'
```

## Call OpenRouter Through TokenGuard

Configure OpenRouter (base **without** trailing `/v1`):

```env
TOKENGUARD_PROVIDER_ROUTES=openrouter=https://openrouter.ai/api
```

Prefer OpenRouter sync for model rates. Optional bootstrap override:

```json
{
  "openrouter/openai/gpt-4o-mini": {
    "input_usd_per_million": 0.15,
    "output_usd_per_million": 0.6
  }
}
```

Request (path is still `/v1/...` on TokenGuard):

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8080/v1/chat/completions `
  -Headers @{
    "Authorization" = "Bearer YOUR_OPENROUTER_API_KEY"
    "X-TokenGuard-API-Key" = "tg_your_tokenguard_key"
    "X-TokenGuard-Provider" = "openrouter"
    "X-TokenGuard-Session-ID" = "session-123"
  } `
  -ContentType "application/json" `
  -Body '{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"Hello"}],"max_tokens":100}'
```

When OpenRouter returns `usage.cost`, TokenGuard prefers that provider-billed USD for settlement.

## Agent Loop Detection

Set a session ID for agent runs:

```http
X-TokenGuard-Session-ID: agent-run-123
```

Without a session ID, loop detection is skipped. With one, TokenGuard hashes the semantic request payload and stores it in Upstash Redis for a short window.

Default settings:

```env
TOKENGUARD_LOOP_TTL_SECONDS=180
TOKENGUARD_LOOP_THRESHOLD=3
```

If the same session sends the same semantic payload 3 times within 3 minutes, TokenGuard blocks it (**409**):

```json
{
  "error": "TokenGuard: Infinite agent loop detected. Circuit breaker tripped to save budget.",
  "code": "loop_detected"
}
```

## Budget Behavior

TokenGuard estimates cost before forwarding:

```text
input token cost + estimated max output token cost
```

If the user cannot afford the estimated cost, TokenGuard returns **402**:

```json
{
  "error": "TokenGuard: budget exceeded",
  "code": "budget_exceeded",
  "available_microusd": 1200,
  "estimated_cost_microusd": 5000,
  "model": "gpt-4o-mini"
}
```

If the request is allowed, TokenGuard reserves the estimated amount, forwards the request, then settles the actual cost after the response (or releases the reservation on provider error / loop block).

Operator extends the limit with `PATCH /mgmt/budget` (see above) or **Users → Edit** in the dashboard.

## Status Codes To Handle

| Code | Meaning |
|------|---------|
| `401` | Missing/invalid `tg_` key (`missing_api_key` / `invalid_api_key`) |
| `400` | Bad body or model not priced (`pricing_not_configured`, …) |
| `402` | Budget exceeded (`budget_exceeded`) |
| `409` | Agent loop detected (`loop_detected`) |
| `413` | Body too large (`request_too_large`) |
| `503` | Billing or Redis unavailable |

## View Users And Usage

List users:

```powershell
Invoke-RestMethod `
  -Method Get `
  -Uri http://127.0.0.1:8080/mgmt/users `
  -Headers @{ "X-TokenGuard-Admin-Secret" = "make-this-at-least-16-chars" }
```

List recent usage:

```powershell
Invoke-RestMethod `
  -Method Get `
  -Uri http://127.0.0.1:8080/mgmt/usage?limit=20 `
  -Headers @{ "X-TokenGuard-Admin-Secret" = "make-this-at-least-16-chars" }
```

List pricing:

```powershell
Invoke-RestMethod `
  -Method Get `
  -Uri http://127.0.0.1:8080/mgmt/pricing `
  -Headers @{ "X-TokenGuard-Admin-Secret" = "make-this-at-least-16-chars" }
```

## Dashboard

With management enabled, open:

```text
http://127.0.0.1:8080/dashboard
```

(On Render: `https://YOUR_SERVICE.onrender.com/dashboard`.)

The dashboard is embedded in the TokenGuard binary (`internal/ui/dashboard.html`), so it works even when the process is not started from the repo root.

Unlock with `TOKENGUARD_ADMIN_SECRET`. From the console you can:

- Provision users (with budget USD)
- Edit / extend budgets (`reset_spent` optional)
- View usage
- Manage pricing (search, upsert, delete, **Sync OpenRouter**)
- Copy integration snippets for this host

## Integration Checklist

For each application using TokenGuard:

- Replace the provider base URL with the TokenGuard URL (`…/v1` for OpenAI-compatible SDKs).
- Keep sending the provider API key using the provider's normal auth header.
- Add `X-TokenGuard-API-Key`.
- Add `X-TokenGuard-Provider` if using multiple providers.
- Add `X-TokenGuard-Session-ID` for agents or long-running workflows.
- Make sure the model exists in the pricing catalog (sync OpenRouter or upsert).
- Handle `401` / `400` / `402` / `409` / `503` in the client.

## Product Summary

TokenGuard helps developers safely use LLM APIs without surprise bills.

It is useful when:

- You are building an AI SaaS product.
- You need per-user (per app / customer) budgets.
- You use autonomous agents.
- You use multiple model providers.
- You want one ledger for AI usage and cost.

Short version:

```text
TokenGuard is a budget firewall for AI apps and agents.
```
