# Integrate TokenGuard

Universal pattern for **any** OpenAI-compatible client, SDK, or agent framework:

```text
BEFORE:  App ──► api.openai.com / openrouter / anthropic
AFTER:   App ──► TokenGuard ──► provider
```

## Integration checklist

1. Deploy TokenGuard and confirm `/healthz`.
2. Provision a user → get `tg_...` key (`/dashboard` or `/mgmt/provision`).
3. Change the **base URL** to your TokenGuard URL.
4. Keep sending the **provider** API key as usual (`Authorization`, `x-api-key`, …).
5. Add `X-TokenGuard-API-Key: tg_...`.
6. Add `X-TokenGuard-Provider` when using multiple providers.
7. Add `X-TokenGuard-Session-ID` for agents / loops.
8. Ensure the model exists in `pricing.json`.

## Headers

| Header | Who sets it | Purpose |
|--------|-------------|---------|
| `Authorization` / `x-api-key` | Your app | Provider auth (passed through) |
| `X-TokenGuard-API-Key` | Your app | Budget identity (`tg_...`) |
| `X-TokenGuard-Provider` | Your app | `openai` / `anthropic` / `openrouter` |
| `X-TokenGuard-Session-ID` | Your app | Loop detection scope |

## OpenAI Node SDK

```js
import OpenAI from "openai";

const client = new OpenAI({
  apiKey: process.env.OPENAI_API_KEY, // still the real provider key
  baseURL: "https://YOUR_SERVICE.onrender.com/v1",
  defaultHeaders: {
    "X-TokenGuard-API-Key": process.env.TOKENGUARD_API_KEY,
    "X-TokenGuard-Provider": "openai",
    "X-TokenGuard-Session-ID": "my-app-session-1",
  },
});

const res = await client.chat.completions.create({
  model: "gpt-4o-mini",
  messages: [{ role: "user", content: "Hello" }],
  max_tokens: 100,
});
```

## OpenAI Python SDK

```python
from openai import OpenAI
import os

client = OpenAI(
    api_key=os.environ["OPENAI_API_KEY"],
    base_url="https://YOUR_SERVICE.onrender.com/v1",
    default_headers={
        "X-TokenGuard-API-Key": os.environ["TOKENGUARD_API_KEY"],
        "X-TokenGuard-Provider": "openai",
        "X-TokenGuard-Session-ID": "my-app-session-1",
    },
)

res = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello"}],
    max_tokens=100,
)
```

## OpenRouter (via TokenGuard)

```js
const client = new OpenAI({
  apiKey: process.env.OPENROUTER_API_KEY,
  baseURL: "https://YOUR_SERVICE.onrender.com",
  defaultHeaders: {
    "X-TokenGuard-API-Key": process.env.TOKENGUARD_API_KEY,
    "X-TokenGuard-Provider": "openrouter",
  },
});
```

Use models that exist in `pricing.json` (e.g. `openai/gpt-4o-mini` or `gpt-4o-mini` depending on how you call the provider).

## Anthropic Messages API

Point the Anthropic client base URL at TokenGuard and keep `x-api-key` + `anthropic-version`. Add TokenGuard headers on each request (or via a custom fetch wrapper).

```text
POST https://YOUR_SERVICE.onrender.com/v1/messages
x-api-key: sk-ant-...
anthropic-version: 2023-06-01
X-TokenGuard-API-Key: tg_...
X-TokenGuard-Provider: anthropic
```

## LangChain / LangGraph / agents

Wherever the LLM client is constructed, set:

- `base_url` / `openai_api_base` → TokenGuard
- default headers → TokenGuard key + session id (use a stable session per agent run)

That is enough for budget + loop protection without changing tools or graphs.

## cURL smoke (after deploy)

```bash
curl -s https://YOUR_SERVICE.onrender.com/healthz

curl -s -X POST https://YOUR_SERVICE.onrender.com/mgmt/provision \
  -H "Content-Type: application/json" \
  -H "X-TokenGuard-Admin-Secret: YOUR_ADMIN_SECRET" \
  -d '{"email":"app@example.com","name":"App"}'
```

## Expected blocks

| Situation | Status |
|-----------|--------|
| Missing/invalid `tg_` key | `401` |
| Unknown model in pricing | `400` |
| Over budget | `402` |
| Agent loop tripped | `409` |
| Turso/Redis down | `503` |

See also: [HOW_TO_USE.md](../HOW_TO_USE.md), [API.md](API.md), [DEPLOY.md](DEPLOY.md)
