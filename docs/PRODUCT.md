# Product

TokenGuard is a **financial firewall for LLM API calls**. It sits between your application and providers such as OpenAI, Anthropic, OpenRouter, Groq, and other OpenAI-compatible APIs.

Apps send requests to TokenGuard instead of directly to the provider. TokenGuard estimates cost, enforces per-user budgets, detects repeated agent loops, records usage, and only then forwards safe requests upstream.

## Problem

AI API bills can grow silently:

- Large prompts sent repeatedly
- Autonomous agents stuck in loops
- Retry bugs that hammer expensive models
- SaaS products that need per-user AI spend caps
- Teams using many providers with no single usage ledger

## What TokenGuard does

- Blocks unsafe requests **before** money is spent
- Enforces per-user budgets (reserve → forward → settle)
- Detects repeated prompt/tool-call loops via Redis
- Tracks input tokens, output tokens, and cost in Turso
- Routes to multiple providers through one proxy

## Who uses it

- AI app developers
- SaaS founders
- Teams building autonomous agents
- Agencies shipping AI products for clients
- Companies that need one ledger across providers

End users do not interact with TokenGuard directly. Developers integrate it as a reverse proxy in their backend.

## Operating modes

| Mode | When | Behavior |
|------|------|----------|
| Proxy-only | `TOKENGUARD_GUARD_ENABLED=false` | Forwards requests; no budget or loop checks (smoke test) |
| Guarded | `TOKENGUARD_GUARD_ENABLED=true` | Requires Turso + Upstash; budgets, usage, loop detection |
| Management | `TOKENGUARD_MGMT_ENABLED=true` | Adds `/mgmt/*` and `/dashboard` (requires guard enabled) |

## Short version

```text
TokenGuard is a budget firewall for AI apps and agents.
```

For setup steps and provider examples, see [HOW_TO_USE.md](../HOW_TO_USE.md).
For request flow and packages, see [ARCHITECTURE.md](ARCHITECTURE.md).
