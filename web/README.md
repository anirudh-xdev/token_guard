# TokenGuard showcase site

Marketing / portfolio site for [TokenGuard](https://github.com/anirudh-xdev/token_guard). Separate from the Go-embedded operator dashboard (`internal/ui`).

## Stack

Next.js (App Router) · Tailwind CSS v4 · deployed independently (e.g. Vercel)

## Develop

```powershell
cd web
copy .env.example .env.local
npm install
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Env

| Variable | Purpose |
|----------|---------|
| `NEXT_PUBLIC_GITHUB_URL` | Repo link (default: anirudh-xdev/token_guard) |
| `NEXT_PUBLIC_DEMO_URL` | Optional live proxy base — enables Live demo / `/healthz` CTAs |

Never put `TOKENGUARD_ADMIN_SECRET` or `tg_` keys in this app.

## Deploy (Vercel)

1. Import the monorepo; set **Root Directory** to `web`.
2. Framework preset: Next.js.
3. Set the env vars above.
4. Deploy. Leave the Go proxy on Render (or elsewhere) unchanged.

```powershell
npm run build
```

## Routes

| Path | Role |
|------|------|
| `/` | Brand hero + problem → solution narrative |
| `/how-it-works` | Guarded request lifecycle |
| `/architecture` | Stores + packages |
| `/docs` | Links into real repo docs |
