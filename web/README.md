# TokenGuard web (Next.js)

Marketing site **and** product portal UI. The Go binary remains the API / proxy.

```text
Browser  →  Next.js (web/)     Clerk sign-in, portal UI
              │
              └─ Bearer JWT  →  Go TokenGuard (:8080)
                                  /portal/api/*, /v1/*, /mgmt/*
```

## Develop

Terminal 1 — API:

```powershell
# repo root
go run ./cmd/tokenguard
```

Terminal 2 — frontend:

```powershell
cd web
copy .env.example .env.local   # fill Clerk + API URL
npm install
npm run dev
```

Open [http://localhost:3000/portal](http://localhost:3000/portal).

## Env (`web/.env.local`)

| Variable | Purpose |
|----------|---------|
| `NEXT_PUBLIC_TOKENGUARD_API_URL` | Go API base (`http://127.0.0.1:8080`) |
| `NEXT_PUBLIC_CLERK_PUBLISHABLE_KEY` | Clerk publishable key |
| `CLERK_SECRET_KEY` | Clerk secret key |
| `NEXT_PUBLIC_DEMO_URL` | Optional marketing live CTAs |
| `NEXT_PUBLIC_GITHUB_URL` | Repo link |

Never put `TOKENGUARD_ADMIN_SECRET` or `tg_` keys in this app.

## Go env (paired)

```env
TOKENGUARD_PORTAL_ENABLED=true
TOKENGUARD_PORTAL_APP_URL=http://localhost:3000/portal
TOKENGUARD_PORTAL_CORS_ORIGINS=http://localhost:3000
TOKENGUARD_CLERK_SECRET_KEY=sk_...
TOKENGUARD_CLERK_PUBLISHABLE_KEY=pk_...
```

In Clerk Dashboard, allow `http://localhost:3000`.

## Routes

| Path | Role |
|------|------|
| `/` | Marketing |
| `/how-it-works` | Lifecycle |
| `/architecture` | Architecture |
| `/docs` | Docs links |
| `/portal` | **Product UI** (Clerk + keys + teams) |
