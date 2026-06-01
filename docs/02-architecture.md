# 02 — Architecture

## System overview

```
        Browser
   ┌──────────────────┐
   │  Vue 3 SPA (Vite) │   HTTP/JSON over /api    ┌────────────────────────┐
   │  Router · fetch   │ ───────────────────────▶ │   Go backend (chi)     │
   │  PoliticianCard   │ ◀─────────────────────── │   handlers · services  │
   └──────────────────┘   Set-Cookie: session     └───────────┬────────────┘
                                                              │
                                          ┌───────────────────┴───────────────────┐
                                          │ pgx pool                              │
                                   ┌──────▼──────┐                        ┌────────▼─────────┐
                                   │ PostgreSQL  │                        │ Wikipedia REST   │
                                   │  (server)   │                        │ (seed time only) │
                                   └─────────────┘                        └──────────────────┘
```

- The **SPA** renders the matchup and leaderboard and talks to the backend only
  through `/api/*` JSON endpoints.
- The **Go backend** owns all logic: pairing, vote recording, Glicko-2 rating
  updates, the contribution gate, and serving the leaderboard. It also serves the
  built SPA static assets in production.
- **PostgreSQL** is the single source of runtime truth (subjects, votes,
  sessions), accessed through a `pgxpool` connection pool.
- **Wikipedia** is contacted only by the **seed/ingestion** step, not on the hot
  path. After seeding, the app needs only PostgreSQL to run.

## Why these choices

| Choice | Why | Alternatives rejected |
|--------|-----|-----------------------|
| Go stdlib `net/http` + **chi** router | Tiny, idiomatic, no heavyweight framework; chi adds clean routing/middleware only. | Gin/Echo (more than needed). |
| **PostgreSQL** via `pgx`/`pgxpool` | Real concurrency and `SERIALIZABLE`/row-locking semantics for consistent rating updates; rich types (`TIMESTAMPTZ`, `BOOLEAN`, `DOUBLE PRECISION`); easy to operate and scale beyond v1. | SQLite (single-writer contention under concurrent voters); in-memory/JSON (no durability, races). |
| **Vue 3 + Vite** | Requested stack; fast dev server, simple SFCs, first-class Router. | Nuxt (SSR unnecessary; SPA is enough). |
| Glicko-2 in-process | Pure function over two ratings (R, RD, σ); no service needed. Models rating *uncertainty* so unproven subjects sort conservatively and converge fast — see [05](05-ranking.md). | Elo (one number, no confidence — replaced); TrueSkill (built for N-player/team games, overkill for strict 1v1). |
| **Stateless signed access token** (24h) as the gate | Enforces R4+R10 without auth (R3); anonymous (no stored record, no identifier) and unforgeable. | Server-stored token (correlatable record, extra lookup); permanent unlock (violates R10). |
| Anonymous cookie session (counter only) | Drives the "you've voted N" UI and pairing variety; non-identifying. | — (kept minimal; **not** the gate). |
| **Wikidata** to enumerate UN leaders; **Wikipedia REST** for summaries | Wikidata gives authoritative, language-independent QIDs for P35/P6 holders; Wikipedia summaries give localized name/description/image. | Hardcoded roster (stale, manual); scraping (brittle). |

## Repository layout

```
e-agora/
├── README.md
├── docs/                      # these specs (source of truth)
│
├── backend/                   # Go module
│   ├── go.mod
│   ├── cmd/
│   │   └── server/
│   │       └── main.go        # wiring, config, graceful shutdown
│   ├── internal/
│   │   ├── http/              # router, handlers, middleware (session, lang, rate, CORS)
│   │   ├── store/             # PostgreSQL access (pgxpool), migrations, queries
│   │   ├── ranking/           # Glicko-2: pure functions + tests
│   │   ├── matchup/           # pair selection logic
│   │   ├── token/             # mint/verify stateless 24h access tokens (HMAC)
│   │   ├── ratelimit/         # per-session token-bucket limiter (R11) + tests
│   │   ├── human/             # humanity check (R12): signed challenge + verify;
│   │   │                      #   pluggable provider (dissent | turnstile | pow)
│   │   ├── lang/              # Accept-Language → Wikipedia code; R9 resolution
│   │   ├── ingest/            # wikidata (enumerate UN leaders) + wikipedia
│   │   │                      #   (per-language summaries) clients + seeder
│   │   ├── subjects/          # add-a-subject (token-gated, 1/token; human + page)
│   │   └── model/             # shared structs (Subject, Translation, Vote, Session)
│   ├── migrations/            # *.sql schema (embedded via embed.FS)
│   └── data/
│       ├── un_leaders.json     # Wikidata snapshot: UN HoS+HoG QIDs (seed input)
│       ├── seed_extra.json     # optional hand-picked humans to seed
│       └── humanity_prompts.json  # rotating pool: loyalty-oath + control prompts (R12)
│
├── docker-compose.yml         # local PostgreSQL for development
│
├── frontend/                  # Vue 3 app
│   ├── package.json
│   ├── vite.config.js         # dev proxy /api → backend
│   ├── index.html
│   └── src/
│       ├── main.js
│       ├── App.vue
│       ├── router/index.js    # routes + leaderboard guard
│       ├── api/client.js      # fetch wrapper (credentials: 'include')
│       ├── stores/            # (optional) contribution state
│       ├── views/
│       │   ├── MatchupView.vue
│       │   └── LeaderboardView.vue
│       └── components/
│           ├── PoliticianCard.vue
│           ├── LeaderboardRow.vue
│           ├── AddSubjectModal.vue   # S3: paste URL / search, validate, add
│           ├── HumanityCheckModal.vue # S4: dissent-based humanity check (R12)
│           ├── AccessBanner.vue      # shows 24h token countdown / re-lock
│           ├── LanguageNote.vue      # "shown in English" fallback notice (R9)
│           └── SiteFooter.vue
│
└── .gitignore
```

> Layout is a target; the [roadmap](07-roadmap.md) builds it incrementally.

## Backend internals

- **Layering**: `http` (transport) → service packages (`matchup`, `ranking`,
  `ingest`) → `store` (persistence). `model` holds plain structs shared across
  layers. Handlers are thin: parse → call service → encode.
- **Migrations**: plain `.sql` files embedded with `//go:embed`; run on startup
  if not yet applied (track a `schema_migrations` table). Idempotent.
- **Sessions**: middleware reads the `eagora_session` cookie; if absent, mints a
  random opaque ID, persists a `sessions` row, and sets the cookie
  (`HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` in prod). Attaches the session
  to the request context. **This is not authentication** — it identifies a
  browser, not a person, solely to count contributions for the UI. It is **not**
  the leaderboard gate.
- **Access tokens** (`token` pkg): the gate (R4+R10). On a successful vote, mint
  a compact signed token — `base64url(payload) "." base64url(HMAC-SHA256(payload))`,
  payload `{iss:"e-agora", iat, exp:iat+24h, jti:<random>}` — using
  `EAGORA_TOKEN_SECRET`. **No subject/session id is included** (anonymity by
  design). Deliver it as cookie `eagora_lb_access` (`HttpOnly`, `SameSite=Lax`,
  `Max-Age=86400`, `Secure` in prod) **and** echo `accessTokenExpiresAt` in the
  vote response for the UI countdown. The leaderboard handler verifies signature
  + `exp` with no DB lookup; invalid/expired → `403`. Secret rotation is the
  kill-switch (stateless tokens aren't individually revocable). The window is
  **fixed**: a vote mints a token only when none is currently valid (extra votes
  don't slide `exp`), so "one add per token" equals "one add per 24h".
- **Add allowance** (R8.1): adding a subject **requires** a valid token and is
  capped at **one per token**. The token's random `jti` is recorded in a tiny
  `subject_add_log` ledger when an add succeeds (insert-on-conflict on `jti`
  claims the single allowance atomically); a second add with the same `jti` →
  `429 add_limit_reached`. The ledger stores only `jti` (+ the added `subject_id`
  and `exp` for cleanup) — **no identifier**, so anonymity holds (it's the only
  server-side token state). Adding never mints/refreshes a token.
- **Language resolution** (`lang` pkg): middleware parses `Accept-Language`
  (primary subtag) and an optional `?lang=` override into a Wikipedia language
  code, attached to context. The matchup handler applies R9 (use the visitor's
  language only if **both** subjects have it, else English).
- **Rate limiting** (`ratelimit` pkg, R11): middleware on mutating endpoints
  (`POST /api/votes`, `POST /api/subjects`) runs **before** any DB work. A
  **token-bucket per `eagora_session`** (capacity `EAGORA_VOTE_BURST`, refill
  `EAGORA_VOTE_RATE`/s) admits brief human bursts and caps sustained throughput;
  on empty bucket → `429 rate_limited` + `Retry-After`. Buckets live **in
  memory** with periodic eviction of idle entries (zero DB overhead, fine for
  the single-instance v1). For horizontal scaling, swap the bucket store for
  Redis/Postgres (same interface). Honest limit: a client that ignores the
  session cookie sidesteps per-session limiting — that case needs an **edge/IP**
  limit (reverse proxy), recommended in prod and out of scope for the app v1.
- **Humanity check** (`human` pkg, R12): voting requires a **human-verified**
  session. The default **`dissent` provider** issues a **stateless signed
  challenge** — `GET /api/human/challenge` returns a prompt from
  `humanity_prompts.json` (randomized option order) plus a `challengeId` that is
  an HMAC-signed envelope encoding the prompt id, the **pass condition**, a
  nonce, and a short `exp` (the correct answer is **never** sent to the client).
  `POST /api/human/verify` checks the signature/expiry and that the submitted
  answer satisfies the pass condition, then sets `sessions.human_verified_until =
  now + EAGORA_HUMAN_TTL`. No challenge table (stateless); the nonce + short exp
  bound replay. One extra signal rides along: a **client interaction-timing
  summary** (decide-latency, instant-click flag, coarse pointer cadence — **no
  typing**) is a *soft* signal that can force another round but **never
  hard-fails alone** (accessibility); timing is evaluated ephemerally and not
  stored. The provider is an **interface** (`dissent`
  default; `turnstile` / `pow` selectable via `EAGORA_HUMAN_PROVIDER`) so a
  stronger/managed check can replace or layer without API changes.
- **Concurrency**: vote handling updates two `subjects` rows + inserts a
  `votes` row + bumps the session counter inside a single PostgreSQL
  transaction. The two `subjects` rows are taken with `SELECT … FOR UPDATE`
  in a deterministic id order (lowest id first) to avoid deadlocks and keep
  ratings consistent under concurrent voters.
- **Config** (env vars): `EAGORA_ADDR` (default `:8080`), `DATABASE_URL`
  (e.g. `postgres://eagora:eagora@localhost:5432/eagora?sslmode=disable`),
  `EAGORA_SEED` (`auto`|`off`|`force`), `EAGORA_SYNC_INTERVAL` (Wikidata refresh
  cadence, default `24h`, `off` to disable), `EAGORA_FALLBACK_LANG` (default `en`),
  `EAGORA_TOKEN_SECRET` (HMAC key for access tokens; **required** in prod),
  `EAGORA_ACCESS_TTL` (default `24h`), `EAGORA_ADDS_PER_TOKEN` (default `1`),
  `EAGORA_VOTE_BURST` (default `20`), `EAGORA_VOTE_RATE` (tokens/sec, default
  `1`), `EAGORA_RATELIMIT` (`on`|`off`, default `on`),
  `EAGORA_HUMAN_PROVIDER` (`dissent`|`turnstile`|`pow`, default `dissent`),
  `EAGORA_HUMAN_TTL` (human-verified window, default `24h`),
  `EAGORA_CORS_ORIGIN` (dev only).

## Frontend internals

- **SPA** with two routes; `LeaderboardView` protected by a navigation guard
  that checks contribution state (fetched from `GET /api/me`). The guard is UX
  only — the server is authoritative (returns `403` if ungated).
- **API client** uses `fetch` with `credentials: 'include'` so the session
  cookie flows on every request.
- **State**: contribution count + unlocked flag kept in a small store (Pinia or
  a reactive module). Re-validated from `/api/me` on load.
- **Styling**: hand-rolled CSS (CSS variables, fl/grid). No component library
  needed for two screens; keep the bundle small.

## Request flows

**Matchup load**
```
GET /api/matchup
  → session + lang middleware (mint session cookie if needed)
  → matchup.SelectPair()          # two distinct active subjects
  → lang.Resolve(visitorLang, A.langs, B.langs)  # R9: visitorLang or "en"
  → store.Translation(id, displayLang) for A,B   # lazy-fetch+cache if missing
  → 200 { a, b, displayLang, fallbackApplied }
```

**Humanity check** (R12)
```
GET  /api/human/challenge
  → human.NewChallenge()   # pick prompt from pool, randomize options
  → 200 { challengeId(signed: promptId, passCond, nonce, exp), prompt, options }
POST /api/human/verify { challengeId, answer, timing }
  → verify signature + exp
  → answer satisfies passCond?  AND  timing not bot-flagged (soft)?
        no  → 200 { verified:false, challengeId(fresh) }
        yes → UPDATE sessions SET human_verified_until = now + EAGORA_HUMAN_TTL
            → 200 { verified:true, humanVerifiedUntil }
```

**Vote**
```
POST /api/votes { winnerId, loserId }
  → session middleware
  → rate-limit middleware (per-session bucket); empty → 429 rate_limited + Retry-After
  → human-verified check: sessions.human_verified_until > now?
        no → 403 human_check_required   # client opens the humanity check (S4)
  → validate pair (both active, distinct, winner≠loser)
  → BEGIN TX
      SELECT … FOR UPDATE both subjects (ordered by id)
      ranking.Update(winner, loser)   # Glicko-2 (R, RD, σ for both)
      store.SaveVote(...)
      store.IncrementSessionContrib(sessionID)
    COMMIT
  → if no valid eagora_lb_access cookie: token.Mint(ttl=24h)  # else keep existing
  → Set-Cookie eagora_lb_access; 200 { contributions, accessTokenExpiresAt, a, b }
```

**Add a subject** (gated, R8.1)
```
POST /api/subjects { url | query | wikidataId }
  → session + rate-limit middleware (per-session bucket) → 429 rate_limited if empty
  → token middleware: require valid eagora_lb_access (else 403 access_required/expired)
  → fast precheck: jti already in subject_add_log? → 429 add_limit_reached
  → resolve → Wikidata QID
  → validate: page exists (R2) AND instance-of human (R8); else 422
  → dedupe by QID (409 if present)
  → fetch available languages + English summary
  → BEGIN TX
       INSERT subject + en translation
       INSERT subject_add_log(jti, subject_id, exp) ON CONFLICT (jti) DO NOTHING
       if 0 rows inserted (raced) → ROLLBACK → 429 add_limit_reached
    COMMIT
  → 201 { subject }     # allowance consumed only on success (claim is atomic w/ insert)
```

**Leaderboard (gated)**
```
GET /api/leaderboard
  → token middleware: verify eagora_lb_access signature + exp
       missing  → 403 { error: "access_required" }
       expired  → 403 { error: "access_expired" }
  → store.TopByRating(limit, offset)   # localized to display language
  → 200 { totalVotes, entries: [{ rank, subject, rating, wins, losses }] }
```

## Deployment shape (v1)

A single Go binary serves JSON **and** the built SPA from an embedded/served
`dist/` directory, talking to a **PostgreSQL** instance over `DATABASE_URL`.
Two processes total (app + database); after the initial Wikipedia seed the app
needs no other external services. In development, PostgreSQL runs via
`docker-compose up db`, and the Vite dev server proxies `/api` to the Go server
(see [roadmap](07-roadmap.md)). The app should run DB migrations on startup and
fail loudly with guidance if `DATABASE_URL` is unreachable.

## Cross-cutting concerns

- **Errors**: JSON envelope `{ "error": "<code>", "message": "<human>" }`;
  codes are stable strings (e.g. `access_required`, `access_expired`,
  `invalid_matchup`, `not_a_person`).
- **Logging**: structured (`slog`); log seed progress, vote throughput, errors.
- **Testing**: `ranking` has unit tests (Glicko-2 is pure — validated against
  Glickman's published worked example); `store` and `http` have
  integration tests against a disposable PostgreSQL (a CI service container or
  `testcontainers-go`).
- **Security posture**: see [04](04-api.md) §Abuse — light mitigations only; this
  is not an authenticated or high-stakes system.
