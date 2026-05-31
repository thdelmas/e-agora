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
| Ranking   | Elo rating (pairwise)                   |

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
| 05 | [Ranking](docs/05-ranking.md) | Elo algorithm and matchup pairing |
| 06 | [Wikipedia ingestion](docs/06-wikipedia-ingestion.md) | How the politician pool is sourced |
| 07 | [Roadmap](docs/07-roadmap.md) | Build milestones and acceptance criteria |

## Status

🚧 **M0 — scaffolding (done).** The repo skeleton is in place and builds:
`backend/` (Go module, chi router, `/api/healthz`, embedded `0001_init.sql`, Elo
package + tests) and `frontend/` (Vue 3 + Vite + Router). API endpoints beyond
`healthz` return `501` until wired in M1–M5. See the
[roadmap](docs/07-roadmap.md) for the build order.

## Local development

```sh
docker compose up -d db          # PostgreSQL on :5432 (matches .env.example)

cd backend && go run ./cmd/server   # serves http://localhost:8080/api/healthz
cd frontend && npm install && npm run dev   # Vite on :5173, proxies /api → :8080
```

Backend tests: `cd backend && go test ./...` (Elo properties pass today).
Copy `.env.example` → `.env` and set `EAGORA_TOKEN_SECRET` before prod.
