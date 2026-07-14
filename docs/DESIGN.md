# Design

Design choices behind TokenGuard. Prefer these invariants when changing code.

## Reserve, then settle

**Why:** Actual output tokens are unknown until the provider responds. Blocking only after the call would still spend money.

**How:** Estimate `input cost + max-output cost`, reserve that amount in `user_budgets.reserved_microusd`, forward the request, then settle actual cost and release the unused reservation.

**Trade-off:** Over-reservation can temporarily reduce available budget. Defaults (`TOKENGUARD_DEFAULT_MAX_OUTPUT_TOKENS`) keep estimates conservative.

## Fail closed on unknown models

**Why:** Guessing price risks underbilling or surprise spend.

**How:** Every allowed model must exist in `pricing.json`. Missing models are blocked.

## Micro-USD integers

**Why:** Avoid floating-point money bugs in a ledger.

**How:** Costs and budgets use integer micro-USD (`1 USD = 1_000_000`). Pricing file fields are micro-USD per 1K tokens.

## Redis for loop detection, not the ledger

**Why:** Loop detection needs short TTL counters with low latency; the financial ledger needs durable rows and constraints.

**How:** Upstash Redis REST stores semantic hash increments. Turso stores users, keys, budgets, and usage events.

**Trade-off:** If Redis is unavailable, guarded mode returns `503` rather than silently allowing loops.

## Env-only configuration

**Why:** One binary, deployable with secrets via environment; no flag sprawl.

**How:** `godotenv` optionally loads `.env`; all knobs come from env vars documented in `.env.example`.

## Strip TokenGuard headers before upstream

**Why:** Provider APIs must not see internal auth or routing headers.

**How:** Proxy removes `X-TokenGuard-*` headers after preflight and before reverse-proxying.

## Management requires guard

**Why:** Provisioning users and listing usage only make sense when the billing store is wired.

**How:** `main` fatals if `TOKENGUARD_MGMT_ENABLED=true` while `TOKENGUARD_GUARD_ENABLED=false`. Admin routes use constant-time compare on `TOKENGUARD_ADMIN_SECRET` (min 16 chars when mgmt is on).

## Hash API keys at rest

**Why:** Plaintext keys must not live in the database.

**How:** Store `key_hash` + short `key_prefix`; return plaintext `tg_...` once at provision time.

## Async usage settlement after response

**Why:** Do not block the client on ledger write latency after a successful upstream call.

**How:** Settlement and usage logging run asynchronously after the response path; failures are logged server-side.

## Single static dashboard

**Why:** Early product needs a simple admin surface without a frontend build pipeline.

**How:** One `internal/ui/dashboard.html` file embedded into the binary via `go:embed`. CORS headers are set only on `/mgmt/*` responses (not on proxy/guard JSON errors).
