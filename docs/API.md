# API

HTTP surface of TokenGuard. Full integrator walkthrough: [HOW_TO_USE.md](../HOW_TO_USE.md).

## Routes

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| `GET` | `/healthz` | None | `{"status":"ok"}` |
| `GET` | `/docs` | None | Public integration guide |
| `GET` | `/v1/tokenguard.json` | None | Discovery (providers, bases, priced models) |
| `GET` | `/dashboard` | None (UI); mgmt secret in UI | Served only if `TOKENGUARD_MGMT_ENABLED=true` |
| `POST` | `/mgmt/provision` | `X-TokenGuard-Admin-Secret` | Create user + `tg_` API key; optional `budget_usd` / `limit_microusd` |
| `PATCH` / `POST` | `/mgmt/budget` | `X-TokenGuard-Admin-Secret` | Set/extend user budget; optional `reset_spent` |
| `GET` | `/mgmt/users` | `X-TokenGuard-Admin-Secret` | List users and budgets |
| `GET` | `/mgmt/usage` | `X-TokenGuard-Admin-Secret` | Recent usage; optional `?limit=` |
| `GET` | `/mgmt/pricing` | `X-TokenGuard-Admin-Secret` | List `model_prices` catalog |
| `POST` / `PUT` | `/mgmt/pricing/upsert` | `X-TokenGuard-Admin-Secret` | Add or update one model price |
| `POST` / `DELETE` | `/mgmt/pricing/delete` | `X-TokenGuard-Admin-Secret` | Delete one model price |
| `POST` | `/mgmt/pricing/sync/openrouter` | `X-TokenGuard-Admin-Secret` | Import prices from OpenRouter models API |
| `*` | `/*` | Provider auth + (if guard) TokenGuard key | Reverse proxy to selected upstream |

Management routes also accept `OPTIONS` for CORS preflight. CORS headers (`Access-Control-Allow-*`) are set only on `/mgmt/*` responses—not on guarded proxy error JSON.

## Client → TokenGuard headers (proxy)

| Header | Required | Purpose |
|--------|----------|---------|
| `X-TokenGuard-API-Key` or `X-TokenGuard-Key` | Yes when guard on | User key (`tg_...`) |
| `X-TokenGuard-Provider` | No | Named provider from `TOKENGUARD_PROVIDER_ROUTES` |
| `X-TokenGuard-Session-ID` | Recommended for agents | Loop detection scope |
| Provider auth (`Authorization`, `x-api-key`, …) | Yes | Passed through to upstream |

TokenGuard strips its own `X-TokenGuard-*` headers before forwarding.

## Admin headers

| Header | Purpose |
|--------|---------|
| `X-TokenGuard-Admin-Secret` | Must match `TOKENGUARD_ADMIN_SECRET` (constant-time compare) |

## Proxy status codes (guard)

| Status | Meaning |
|--------|---------|
| `401` | Missing or invalid TokenGuard API key |
| `400` | Bad request body / analysis failure / unknown model pricing |
| `413` | Body exceeds `TOKENGUARD_MAX_REQUEST_BYTES` |
| `402` | Budget exceeded — operator can `PATCH /mgmt/budget` to extend |
| `409` | Agent loop circuit breaker tripped |
| `503` | Billing store or loop breaker unavailable |
| `502` | Upstream / proxy failure |

Successful forwards return the **upstream** status and body.

## Example: provision

```http
POST /mgmt/provision
Content-Type: application/json
X-TokenGuard-Admin-Secret: your-admin-secret

{"email":"dev@example.com","name":"Dev User","budget_usd":50}
```

```json
{
  "user_id": "user_xxx",
  "api_key": "tg_xxx",
  "api_key_id": "key_xxx",
  "limit_microusd": 50000000,
  "budget_usd": 50,
  "integration": { "docs_url": "/docs", "proxy_url": "/v1/chat/completions" }
}
```

Default budget when omitted: **$1.00** (`1_000_000` micro-USD).

## Example: extend budget (after 402)

```http
PATCH /mgmt/budget
Content-Type: application/json
X-TokenGuard-Admin-Secret: your-admin-secret

{"user_id":"user_xxx","budget_usd":100,"reset_spent":false}
```

Set `reset_spent: true` to zero spent when starting a fresh period after raising the limit.

## Example: upsert model price

```http
POST /mgmt/pricing/upsert
Content-Type: application/json
X-TokenGuard-Admin-Secret: your-admin-secret

{"model_key":"gpt-4o-mini","input_cost_per_1k":150,"output_cost_per_1k":600}
```

Costs are **micro-USD per 1K tokens**. Live catalog lives in Turso `model_prices` (seeded from `pricing.json` on first empty boot).

## OpenRouter base URL

Use upstream base `https://openrouter.ai/api` (not `.../api/v1`). Clients call `/v1/chat/completions` on TokenGuard; joining `.../api/v1` + `/v1/...` produces `/api/v1/v1/...` and upstream 404s. TokenGuard normalizes the known misconfig on load. Discovery (`/v1/tokenguard.json`) exposes effective `provider_bases` (no secrets).

## Example: chat completion via proxy

```http
POST /v1/chat/completions
Authorization: Bearer sk-...
X-TokenGuard-API-Key: tg_xxx
X-TokenGuard-Provider: openai
X-TokenGuard-Session-ID: session-123
Content-Type: application/json

{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}],"max_tokens":100}
```

## Budget block body (example)

```json
{
  "error": "TokenGuard: budget exceeded",
  "available_microusd": 1200,
  "estimated_cost_microusd": 5000,
  "model": "gpt-4o-mini"
}
```

## Loop block body (example)

```json
{
  "error": "TokenGuard: Infinite agent loop detected. Circuit breaker tripped to save budget."
}
```
