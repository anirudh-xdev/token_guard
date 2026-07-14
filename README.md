# TokenGuard

Financial firewall for LLM API calls. Sit TokenGuard between your app and providers (OpenAI, Anthropic, OpenRouter, Groq, and other OpenAI-compatible APIs). It estimates cost, enforces per-user budgets, detects agent loops, tracks usage, and only then forwards safe requests.

```text
Your App → TokenGuard → budget / loop / cost checks → Provider
```

## Quick start

### Smoke test (no Turso / Redis)

```powershell
$env:TOKENGUARD_GUARD_ENABLED='false'
$env:TOKENGUARD_LISTEN_ADDR='127.0.0.1:18080'
$env:TIKTOKEN_CACHE_DIR='.tiktoken-cache'

go run ./cmd/tokenguard
```

```powershell
Invoke-WebRequest http://127.0.0.1:18080/healthz -UseBasicParsing
```

### Guarded mode

1. Copy [`.env.example`](.env.example) to `.env` and fill Turso + Upstash credentials.
2. Ensure models you use exist in [`pricing.json`](pricing.json).
3. Build and run:

```powershell
go build -o tokenguard.exe ./cmd/tokenguard
.\tokenguard.exe
```

4. Provision a user (mgmt enabled) and call providers through TokenGuard — see [HOW_TO_USE.md](HOW_TO_USE.md).

## Documentation

| Doc | Description |
|-----|-------------|
| [HOW_TO_USE.md](HOW_TO_USE.md) | Full setup, providers, budgets, dashboard |
| [docs/INDEX.md](docs/INDEX.md) | Docs map |
| [docs/PRODUCT.md](docs/PRODUCT.md) | What it is for and who uses it |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | Request flow and packages |
| [docs/DESIGN.md](docs/DESIGN.md) | Design decisions |
| [docs/STACK.md](docs/STACK.md) | Tech stack |
| [docs/STRUCTURE.md](docs/STRUCTURE.md) | Repo layout |
| [docs/API.md](docs/API.md) | HTTP routes and headers |
| [docs/DEPLOY.md](docs/DEPLOY.md) | Deploy on Render |
| [docs/INTEGRATION.md](docs/INTEGRATION.md) | Connect SDKs and apps |
| [AGENTS.md](AGENTS.md) | Guidance for coding agents |

## For developers (after deploy)

| URL | Purpose |
|-----|---------|
| `/docs` | Public how-to (no secret) |
| `/dashboard` | Developer console (admin secret) |
| `/v1/tokenguard.json` | Machine-readable discovery |
| `/healthz` | Liveness |

## Stack (short)

Go 1.21 · Turso (libSQL) · Upstash Redis REST · tiktoken · embedded `internal/ui/dashboard.html`

## License

See repository owners for licensing terms.
