# TokenGuard: How To Use It

TokenGuard is a financial firewall for AI apps. It sits between your application and LLM providers such as OpenAI, Anthropic Claude, OpenRouter, Groq, Mistral, and other OpenAI-compatible APIs.

For product overview, architecture, and agent guidance, see [docs/INDEX.md](docs/INDEX.md), [README.md](README.md), and [AGENTS.md](AGENTS.md).

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
- Enforcing per-user budgets.
- Detecting repeated prompt/tool-call loops.
- Tracking input tokens, output tokens, and cost.
- Supporting multiple providers through one proxy.

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

If a request is too expensive, over budget, or looks like an agent loop, TokenGuard blocks it with a JSON error.

## Who Uses TokenGuard

TokenGuard is mainly for:

- AI app developers
- SaaS founders
- Teams building autonomous agents
- Agencies building AI products for clients
- Companies using multiple LLM providers

The end user does not usually interact with TokenGuard directly. Developers integrate it into their backend as a proxy.

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

## Real Guarded Mode

Guarded mode enables budgets, usage logging, and loop detection.

Create a `.env` file using `.env.example` as the template.

Minimum required values:

```env
TOKENGUARD_GUARD_ENABLED=true
TOKENGUARD_LISTEN_ADDR=:8080

TURSO_DATABASE_URL=libsql://your-database.turso.io
TURSO_AUTH_TOKEN=your_turso_token

UPSTASH_REDIS_REST_URL=https://your-redis-instance.upstash.io
UPSTASH_REDIS_REST_TOKEN=your_upstash_token

TOKENGUARD_PRICING_FILE=pricing.json
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
- Load `pricing.json`.
- Start the proxy server.

## Configure Multiple Providers

Use `TOKENGUARD_PROVIDER_ROUTES` to define named providers.

Example:

```env
TOKENGUARD_PROVIDER_ROUTES=anthropic=https://api.anthropic.com,openrouter=https://openrouter.ai/api/v1,groq=https://api.groq.com/openai/v1
```

Supported style:

```text
provider_name=https://provider-api-base-url
```

Then choose a provider per request with:

```http
X-TokenGuard-Provider: anthropic
```

If no provider is specified, TokenGuard uses `TOKENGUARD_DEFAULT_PROVIDER`.

## Pricing Setup

Every model that TokenGuard allows must exist in `pricing.json`.

Basic format:

```json
{
  "gpt-4o-mini": {
    "input_cost_per_1k": 150,
    "output_cost_per_1k": 600
  }
}
```

Costs are stored in micro-USD.

```text
1 USD = 1,000,000 micro-USD
```

You can also use provider-scoped model names:

```json
{
  "anthropic/claude-3-5-sonnet-latest": {
    "input_cost_per_1k": 3000,
    "output_cost_per_1k": 15000
  },
  "openrouter/meta-llama/llama-3.1-70b-instruct": {
    "input_cost_per_1k": 900,
    "output_cost_per_1k": 900
  }
}
```

If a model is missing from `pricing.json`, TokenGuard blocks the request instead of guessing.

## Provision A User

Management endpoints must be enabled:

```env
TOKENGUARD_MGMT_ENABLED=true
TOKENGUARD_ADMIN_SECRET=make-this-at-least-16-chars
```

Create a user and TokenGuard API key:

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8080/mgmt/provision `
  -Headers @{ "X-TokenGuard-Admin-Secret" = "make-this-at-least-16-chars" } `
  -ContentType "application/json" `
  -Body '{"email":"dev@example.com","name":"Dev User"}'
```

Response:

```json
{
  "user_id": "user_xxx",
  "api_key": "tg_xxx",
  "api_key_id": "key_xxx"
}
```

The `tg_xxx` key is the TokenGuard key your application sends with each request.

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

TokenGuard forwards `Authorization` to OpenAI but strips its own internal headers before forwarding.

## Call Claude Through TokenGuard

Configure Anthropic:

```env
TOKENGUARD_PROVIDER_ROUTES=anthropic=https://api.anthropic.com
```

Add Anthropic model pricing to `pricing.json`:

```json
{
  "anthropic/claude-3-5-sonnet-latest": {
    "input_cost_per_1k": 3000,
    "output_cost_per_1k": 15000
  }
}
```

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
  -Body '{"model":"claude-3-5-sonnet-latest","max_tokens":100,"messages":[{"role":"user","content":"Hello"}]}'
```

## Call OpenRouter Through TokenGuard

Configure OpenRouter:

```env
TOKENGUARD_PROVIDER_ROUTES=openrouter=https://openrouter.ai/api/v1
```

Add pricing:

```json
{
  "openrouter/openai/gpt-4o-mini": {
    "input_cost_per_1k": 150,
    "output_cost_per_1k": 600
  }
}
```

Request:

```powershell
Invoke-RestMethod `
  -Method Post `
  -Uri http://127.0.0.1:8080/chat/completions `
  -Headers @{
    "Authorization" = "Bearer YOUR_OPENROUTER_API_KEY"
    "X-TokenGuard-API-Key" = "tg_your_tokenguard_key"
    "X-TokenGuard-Provider" = "openrouter"
    "X-TokenGuard-Session-ID" = "session-123"
  } `
  -ContentType "application/json" `
  -Body '{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"Hello"}],"max_tokens":100}'
```

## Agent Loop Detection

Set a session ID for agent runs:

```http
X-TokenGuard-Session-ID: agent-run-123
```

TokenGuard hashes the semantic request payload and stores it in Upstash Redis for a short window.

Default settings:

```env
TOKENGUARD_LOOP_TTL_SECONDS=180
TOKENGUARD_LOOP_THRESHOLD=3
```

If the same session sends the same semantic payload 3 times within 3 minutes, TokenGuard blocks it:

```json
{
  "error": "TokenGuard: Infinite agent loop detected. Circuit breaker tripped to save budget."
}
```

## Budget Behavior

TokenGuard estimates cost before forwarding:

```text
input token cost + estimated max output token cost
```

If the user cannot afford the estimated cost, TokenGuard returns:

```http
402 Payment Required
```

Example response:

```json
{
  "error": "TokenGuard: budget exceeded",
  "available_microusd": 1200,
  "estimated_cost_microusd": 5000,
  "model": "gpt-4o-mini"
}
```

If the request is allowed, TokenGuard reserves the estimated amount, forwards the request, then settles the actual cost after the response.

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

## Dashboard

With management enabled, open:

```text
http://127.0.0.1:8080/dashboard
```

The dashboard is embedded in the TokenGuard binary (see `internal/ui/dashboard.html`), so it works even when the process is not started from the repo root.

The page asks for your admin secret and uses the management endpoints to show users and recent usage.

If you need to point the UI at a different API base (rare when using `/dashboard` on the same origin), set this in the browser console:

```javascript
localStorage.setItem("tokenguard_api_base", "http://127.0.0.1:8080")
```

Then refresh the page.

## Integration Checklist

For each application using TokenGuard:

- Replace the provider base URL with the TokenGuard URL.
- Keep sending the provider API key using the provider's normal auth header.
- Add `X-TokenGuard-API-Key`.
- Add `X-TokenGuard-Provider` if using multiple providers.
- Add `X-TokenGuard-Session-ID` for agents or long-running workflows.
- Make sure the model exists in `pricing.json`.

## Product Summary

TokenGuard helps developers safely use LLM APIs without surprise bills.

It is useful when:

- You are building an AI SaaS product.
- You need per-user budgets.
- You use autonomous agents.
- You use multiple model providers.
- You want one ledger for AI usage and cost.

Short version:

```text
TokenGuard is a budget firewall for AI apps and agents.
```
