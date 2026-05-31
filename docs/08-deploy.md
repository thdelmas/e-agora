# 08 — Deploy (free: Neon + Render)

e-agora deploys as **one container** — [`Dockerfile.prod`](../Dockerfile.prod)
builds the Vue SPA and the Go API into a single image that serves both
same-origin on port `8080` (the API under `/api`, the SPA everywhere else, with
a client-side-routing fallback). So a full deployment is just:

> **1 web service** (the container) **+ 1 Postgres** + your domain pointed at it.

This guide uses a **free Neon Postgres** (persistent) and a **free Render web
service** (free TLS + custom domain). Both have genuine free tiers with no card
required. The repo's [`render.yaml`](../render.yaml) wires the service up for
you.

Trade-offs to know up front:

- **Cold starts** — a Render free service spins down after ~15 min idle and
  takes ~1 min to wake; Neon scales its compute to zero and wakes in well under
  a second. Fine for a hobby/portfolio site, not for low-latency production.
- **Don't use Render's own free Postgres** — it expires 30 days after creation
  and is then deleted. Neon's free tier is persistent (0.5 GB), which is why we
  pair the two.

---

## 1. Create the database (Neon)

1. Sign up at [neon.tech](https://neon.tech) and create a project. Pick a region
   close to where you'll run Render (e.g. **Frankfurt** for the EU) — keeping the
   app and DB in the same region keeps query latency low.
2. In the project's **Connection Details**, copy the **direct** connection
   string (the one *without* `-pooler` in the host). It looks like:

   ```
   postgresql://USER:PASSWORD@ep-xxxx.eu-central-1.aws.neon.tech/neondb?sslmode=require
   ```

   Make sure it ends with `?sslmode=require` — Neon only accepts TLS connections.

   > **Why the direct string, not the pooled one?** e-agora runs as a
   > long-lived container with its own `pgxpool`, so it doesn't need Neon's
   > PgBouncer pooler. The direct endpoint also avoids pgx-v5's prepared-statement
   > incompatibility with PgBouncer's transaction-pooling mode.

Migrations are embedded in the binary and run automatically on boot
([`backend/migrations`](../backend/migrations)), so there's nothing to apply by
hand — the first deploy creates the schema in the empty Neon database.

## 2. Deploy the app (Render)

1. Push this repo to GitHub (it already lives at `thdelmas/e-agora`).
2. In the [Render dashboard](https://dashboard.render.com): **New → Blueprint**,
   select the repo. Render reads [`render.yaml`](../render.yaml) and creates a
   free Docker web service that builds from `Dockerfile.prod`.
3. When prompted, paste the Neon string from step 1 into **`DATABASE_URL`**.
   `EAGORA_TOKEN_SECRET` is generated for you; `EAGORA_SEED=auto` is preset.
4. Click apply. The first build compiles the SPA + Go binary and deploys.

> Adjust the `region:` in `render.yaml` to match your Neon region if you didn't
> use Frankfurt.

If you'd rather not use the blueprint, create a **Web Service** manually from
the repo: runtime **Docker**, Dockerfile path `./Dockerfile.prod`, plan
**Free**, health check path `/api/healthz`, and set the three env vars above by
hand (generate a strong random `EAGORA_TOKEN_SECRET` yourself).

## 3. Seeding the pool

`EAGORA_SEED=auto` makes the server seed the UN-leaders pool from
Wikidata/Wikipedia in a background goroutine on first boot, then short-circuit on
every later boot (the pool is already non-empty). Watch it land:

```
https://<your-service>.onrender.com/api/healthz   →  {"subjects": N, "seeded": true}
```

`subjects` climbs from 0 to ~195 over a few minutes. Keep the tab open until
`seeded` flips true so the free instance doesn't spin down mid-seed.

**More reliable alternative** (recommended if the seed gets interrupted): seed
once from your machine against Neon, then turn auto-seed off so the deploy never
depends on a background fetch:

```sh
cd backend
DATABASE_URL='postgresql://…neon…?sslmode=require' \
EAGORA_TOKEN_SECRET=anything \
EAGORA_SEED=force \
go run ./cmd/server          # let it finish seeding, then Ctrl-C
```

Then set `EAGORA_SEED=off` on the Render service (Environment → edit) and
redeploy. The data already lives in Neon, so boots are instant and never touch
the upstreams.

## 4. Your domain + HTTPS

1. Render service → **Settings → Custom Domains → Add Custom Domain**, enter your
   domain (apex like `e-agora.org`, or a subdomain like `www`).
2. Add the DNS record Render shows you at your registrar:
   - **subdomain** (`www`) → a `CNAME` to `<your-service>.onrender.com`
   - **apex/root** (`e-agora.org`) → an `ALIAS`/`ANAME` (or the `A` record Render
     provides) to the same target
3. Render verifies the record and issues a free Let's Encrypt certificate
   automatically (a few minutes). Done — one origin serves both the app and the
   API over HTTPS, so there's no second host, no CORS, and no second domain to
   manage.
4. **Set `EAGORA_PUBLIC_URL`** to that origin (e.g. `https://e-agora.org`, no
   trailing slash) so the canonical/Open Graph tags, `robots.txt`, and
   `sitemap.xml` all point at the one host. Leaving it blank works (the backend
   derives the origin per request) but lets the `*.onrender.com` URL and your
   domain self-canonicalise as duplicates — set it once the domain is live.

## 5. Verify

- `https://your-domain/api/healthz` → `200` with `seeded: true`.
- `https://your-domain/` → the app loads; cast a vote, which unlocks the
  leaderboard (the 24h access gate) — confirming the API, DB, and SPA are all
  wired same-origin.
- `https://your-domain/robots.txt` and `/sitemap.xml` → return text/XML that
  reference your domain (or `EAGORA_PUBLIC_URL` if set). `curl -s
  https://your-domain/ | grep canonical` should show the canonical/OG tags.

## Environment variables (production)

| Variable | Value | Notes |
|----------|-------|-------|
| `DATABASE_URL` | Neon **direct** string + `?sslmode=require` | set in Render (`sync: false`) |
| `EAGORA_TOKEN_SECRET` | strong random secret | Render generates it; required to boot |
| `EAGORA_SEED` | `auto` (or `off` after a local seed) | first-boot ingestion |
| `EAGORA_STATIC_DIR` | `/web` | already baked into `Dockerfile.prod` |
| `EAGORA_PUBLIC_URL` | `https://your-domain` (no trailing slash) | canonical origin for SEO tags, `robots.txt`, `sitemap.xml`; blank → derived per request |
| `EAGORA_ADDR` | `:8080` (default) | matches the image's `EXPOSE`; Render detects it |

All other tunables ([`config.go`](../backend/internal/config/config.go)) keep
their documented defaults.

## Alternatives

The same single-image build runs unchanged on other free hosts if Render's cold
starts bother you — **Google Cloud Run** (always-free request allowance, wakes
faster, default `$PORT` is `8080`) or **Fly.io** (closer to always-on, free TLS;
free allowance is trial-credit-based), each paired with the same Neon database.
Splitting the SPA onto a static host (Pages/Netlify/Vercel) would reintroduce
CORS and a second origin, undoing the same-origin design — not worth it here.
