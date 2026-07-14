# Deploy TokenGuard

TokenGuard is a single web service. Keep **Turso** and **Upstash** as external dependencies (already in your `.env`).

Recommended host: **Render** (Docker). Repo: `https://github.com/anirudh-xdev/token_guard.git`

## What you need

| Piece | Source |
|-------|--------|
| App binary | This repo (`Dockerfile`) |
| Users / budgets / usage | Turso (`TURSO_*`) |
| Loop breaker | Upstash Redis REST (`UPSTASH_*`) |
| Model prices | `pricing.json` (copied into the image) |

## Flow

```text
1. Push deploy files to GitHub
2. Create Render Web Service from repo (Docker)
3. Paste secret env vars from .env
4. Wait for deploy → GET https://<service>.onrender.com/healthz
5. Open /dashboard → provision a user
6. Point your app base URL at TokenGuard (see INTEGRATION.md)
```

## Option A — Blueprint (`render.yaml`)

1. Commit and push `Dockerfile`, `render.yaml`, and PORT support.
2. In [Render Dashboard](https://dashboard.render.com/) → **New** → **Blueprint**.
3. Connect `anirudh-xdev/token_guard`.
4. When prompted, fill secrets (`sync: false` keys):
   - `TURSO_DATABASE_URL`
   - `TURSO_AUTH_TOKEN`
   - `UPSTASH_REDIS_REST_URL`
   - `UPSTASH_REDIS_REST_TOKEN`
   - `TOKENGUARD_ADMIN_SECRET` (16+ chars, strong in production)
5. Deploy.

## Option B — Manual Web Service

1. **New** → **Web Service** → this GitHub repo.
2. Runtime: **Docker**.
3. Health check path: `/healthz`.
4. Add the same env vars as in `render.yaml` (+ secrets).
5. Do **not** set `TOKENGUARD_LISTEN_ADDR` on Render — the app binds `0.0.0.0:$PORT` automatically.

## After deploy checklist

```text
GET  https://YOUR_SERVICE.onrender.com/healthz
     → {"status":"ok"}

GET  https://YOUR_SERVICE.onrender.com/dashboard
     → admin UI (enter TOKENGUARD_ADMIN_SECRET)

POST https://YOUR_SERVICE.onrender.com/mgmt/provision
     Header: X-TokenGuard-Admin-Secret
     Body: {"email":"...","name":"..."}
     → save the tg_ API key
```

## Free-tier note

Render free web services sleep after ~15 minutes idle. First request after sleep can take 30–60s. Fine for demos; use a paid plan for production agents.

## Local Docker smoke

```powershell
docker build -t tokenguard .
docker run --rm -p 8080:8080 --env-file .env -e PORT=8080 tokenguard
```

Next: [INTEGRATION.md](INTEGRATION.md)
