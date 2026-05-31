# e-agora

> A worldwide ranking of political figures, decided one head-to-head at a time.

**e-agora** is a small full-stack web app. A visitor lands, is asked *"Who do you
prefer between A and B?"*, picks one, and — only after contributing that vote —
unlocks a live leaderboard. Rankings are computed purely from accumulated
pairwise preferences. Every politician in the pool is backed by a **Wikipedia
page**.

The name nods to the *agora* of ancient Greece — the public square where the
*polis* deliberated — rebuilt as an electronic one (*e-*).

## The five hard requirements

These are the non-negotiable constraints the product is built around:

1. **Worldwide political-figures ranking** — the pool **seeds with every
   UN-recognized country's leaders** (head of state + head of government), and
   visitors can **add any human** who has a Wikipedia page. Every entry **must**
   have a Wikipedia page (the source of truth for identity, description, image).
2. **No user authentication** — no signup, no login, no passwords. (Voting *is*
   gated by an anonymous **humanity check** so bots can't vote — proving "a
   human," not *who*; that's not authentication.)
3. **Contribution before consumption** — a visitor must vote before viewing the
   leaderboard. A vote grants a **24-hour access token**; when it lapses, the
   leaderboard re-locks until they vote again.
4. **Simple flow** — land → "who do you prefer, A or B?" → vote → see the
   leaderboard. Matchups render in the **visitor's language** when both subjects
   have it, otherwise English. Ranking reflects who is preferred.
5. **Project name** — *e-agora*.

## Stack

| Layer     | Choice                                  |
|-----------|-----------------------------------------|
| Frontend  | Vue 3 + Vite + Vue Router               |
| Backend   | Go (standard library HTTP + chi router) |
| Storage   | PostgreSQL (via `pgx` / `pgxpool`)      |
| Data      | Wikidata (enumerate UN leaders, "is a human") + Wikipedia REST (multilingual summary + image) |
| i18n      | Per-language summaries, anchored on Wikidata QIDs |
| Gate      | Stateless signed 24h access token (anonymous) |
| Anti-abuse| Humanity check (bots can't vote) + per-session vote rate limit; one add per token |
| Ranking   | Glicko-2 rating (pairwise, with uncertainty) |

## Documentation

Specs live in [`docs/`](docs/) and are the source of truth for the build. Read
them in order:

| # | Document | What it covers |
|---|----------|----------------|
| 00 | [Overview](docs/00-overview.md) | Vision, requirements, glossary, open questions |
| 01 | [Functional spec](docs/01-functional-spec.md) | User flows, screens, the contribution gate |
| 02 | [Architecture](docs/02-architecture.md) | Components, tech choices, repo layout |
| 03 | [Data model](docs/03-data-model.md) | PostgreSQL schema and entities |
| 04 | [API](docs/04-api.md) | REST contract between frontend and backend |
| 05 | [Ranking](docs/05-ranking.md) | Glicko-2 algorithm and matchup pairing |
| 06 | [Wikipedia ingestion](docs/06-wikipedia-ingestion.md) | How the politician pool is sourced |
| 07 | [Roadmap](docs/07-roadmap.md) | Build milestones and acceptance criteria |
| 08 | [Deploy](docs/08-deploy.md) | Free hosting: Neon Postgres + Render, custom domain |

## Status

🚧 **Building.** M0 (scaffolding) and M1 (DB + migrations + `/api/healthz`) are
done. **M2 (ingestion)** is in progress: the pool seeds on first boot from a
committed Wikidata snapshot of UN-country leaders (head of state + head of
government), enriched via the Wikipedia summary API. Endpoints beyond `healthz`
return `501` until wired in M3–M5. See the [roadmap](docs/07-roadmap.md).

## Local development

Whole stack in Docker:

```sh
make dev    # build + run db + backend + frontend
# frontend → http://localhost:5173   backend → http://localhost:8080/api/healthz
```

…or run pieces on the host:

```sh
docker compose up -d db                     # just PostgreSQL on :5432
cd backend && go run ./cmd/server           # http://localhost:8080/api/healthz
cd frontend && npm install && npm run dev   # Vite on :5173, proxies /api → :8080
```

`make help` lists tasks; `make test` runs backend tests. Copy `.env.example` →
`.env` and set `EAGORA_TOKEN_SECRET` before prod. First boot seeds the pool from
Wikidata/Wikipedia in the background (`EAGORA_SEED=off` to skip, `force` to
re-ingest).

## Production

One image serves the API **and** the SPA same-origin; it needs only a
PostgreSQL alongside it.

```sh
make prod-build                              # → e-agora:latest (SPA + backend)
docker run -p 8080:8080 \
  -e DATABASE_URL='postgres://…' \
  -e EAGORA_TOKEN_SECRET='<a strong secret>' \
  e-agora:latest                             # serves the app on :8080
```

`EAGORA_TOKEN_SECRET` is **required** — the server refuses to boot without it
(it signs the access token and humanity challenges). The image sets
`EAGORA_STATIC_DIR=/web`; point it elsewhere to serve a different build. When
`EAGORA_STATIC_DIR` is unset (dev), Vite serves the SPA instead.

To host it **for free** (Neon Postgres + Render, with a custom domain over
HTTPS), follow [docs/08-deploy.md](docs/08-deploy.md) — the repo's
[`render.yaml`](render.yaml) deploys this same image in a few clicks.
