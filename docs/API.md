# API

HTTP surface of TokenGuard. Full integrator walkthrough: [HOW_TO_USE.md](../HOW_TO_USE.md).

## Routes

| Method | Path | Auth | Notes |
|--------|------|------|-------|
| `GET` | `/healthz` | None | `{"status":"ok"}` |
| `GET` | `/dashboard` | None (UI); mgmt secret in UI | Served only if `TOKENGUARD_MGMT_ENABLED=true` |
| `POST` | `/mgmt/provision` | `X-TokenGuard-Admin-Secret` | Create user + `tg_` API key |
| `GET` | `/mgmt/users` | `X-TokenGuard-Admin-Secret` | List users and budgets |
| `GET` | `/mgmt/usage` | `X-TokenGuard-Admin-Secret` | Recent usage; optional `?limit=` |
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
| `402` | Budget exceeded or related payment block |
| `409` | Agent loop circuit breaker tripped |
| `503` | Billing store or loop breaker unavailable |
| `502` | Upstream / proxy failure |

Successful forwards return the **upstream** status and body.

## Example: provision

```http
POST /mgmt/provision
Content-Type: application/json
X-TokenGuard-Admin-Secret: your-admin-secret

{"email":"dev@example.com","name":"Dev User"}
```

```json
{
  "user_id": "user_xxx",
  "api_key": "tg_xxx",
  "api_key_id": "key_xxx"
}
```

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
