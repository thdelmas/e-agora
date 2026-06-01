# 03 — Data Model

Storage is **PostgreSQL**. The pool is anchored on **Wikidata QIDs** (so the
same person is one row across all language editions, R9/D7), with localized
display content kept in a separate, lazily-filled **translations** table.

Six tables: `subjects`, `subject_translations`, `votes`, `sessions`,
`subject_add_log`, and the `schema_migrations` bookkeeping table. The 24h access
token (R10) is **stateless and signed** — the token itself is **never stored**;
the only token-related state is `subject_add_log`, an anonymous record of which
token `jti`s have spent their one-add allowance (R8.1), holding **no**
identifier (see [04](04-api.md)/[02](02-architecture.md)).

Types are Postgres-native: `BIGINT … GENERATED ALWAYS AS IDENTITY` keys,
`TIMESTAMPTZ` timestamps, `BOOLEAN` flags, `DOUBLE PRECISION` ratings, and
`TEXT[]` for the set of available languages.

## Entity-relationship

```
  sessions (anon counter)         subjects (the pool, keyed by Wikidata QID)
  ┌────────────────────┐          ┌──────────────────────────────────────┐
  │ id (PK, opaque)    │          │ id (PK)                              │
  │ contributions      │          │ wikidata_id (UQ)  ← identity anchor  │
  │ created_at         │          │ canonical_name, source               │
  │ last_seen_at       │          │ available_langs TEXT[]               │
  └─────────┬──────────┘          │ rating, rd, volatility,              │
            │                     │ wins, losses, comparisons,           │
            │                     │ active, created_at, updated_at       │
            │ 1                   └───┬───────────────────────┬──────────┘
            │                      1  │                   2 (winner,loser)
            │ N                       │ N                     │ N
          ┌─▼─────────────────────────┼──────────────────────▼─┐
          │ votes                      │                         │
          │ id (PK) · session_id (FK)  │  subject_translations   │
          │ winner_id (FK→subjects)    │  (subject_id, lang) PK   │
          │ loser_id  (FK→subjects)    │  name, description,      │
          │ winner/loser_rating_before │  image_url, wikipedia_url│
          │ winner/loser_rd_before     │  fetched_at              │
          │ winner/loser_vol_before    │  FK→subjects(id)         │
          │ created_at                 │                          │
          └────────────────────────────┴──────────────────────────┘
```

## Tables

### `subjects`

The ranked pool — language-**neutral** core. Seeded with UN-country leaders
(R1.1) and grown by visitors with any human (R8). Every row corresponds to a
real Wikidata entity that has at least one Wikipedia page (R2).

| Column | Type | Notes |
|--------|------|-------|
| `id` | BIGINT IDENTITY PK | internal id |
| `wikidata_id` | TEXT NOT NULL UNIQUE | e.g. `Q567`; **identity anchor & dedup key** |
| `canonical_name` | TEXT NOT NULL | default (English) label; for logs/admin/fallback |
| `source` | TEXT NOT NULL DEFAULT `'seed'` | `'seed'` (UN leaders) or `'user'` (R8) |
| `available_langs` | TEXT[] NOT NULL DEFAULT `'{}'` | Wikipedia language codes with a page (drives R9 without remote checks) |
| `rating` | DOUBLE PRECISION NOT NULL DEFAULT 1500 | Glicko-2 rating; ranking key |
| `rd` | DOUBLE PRECISION NOT NULL DEFAULT 350 | Glicko-2 rating deviation (uncertainty); shrinks with evidence |
| `volatility` | DOUBLE PRECISION NOT NULL DEFAULT 0.06 | Glicko-2 volatility (σ); how erratic results have been |
| `wins` | INTEGER NOT NULL DEFAULT 0 | times chosen as winner |
| `losses` | INTEGER NOT NULL DEFAULT 0 | times chosen as loser |
| `comparisons` | INTEGER NOT NULL DEFAULT 0 | `wins + losses`; denormalized |
| `active` | BOOLEAN NOT NULL DEFAULT TRUE | FALSE hides from matchups & leaderboard (moderation handle) |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |
| `updated_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | bumped on rating change |

**Invariants**
- `wikidata_id` unique → one person = one row across languages (R9).
- a subject is only created if it resolves to a Wikidata human (R8) with ≥1
  Wikipedia page (R2); `'en' = ANY(available_langs)` for all seed leaders and
  effectively all added humans (English is the R9 fallback — see Overview Q6).
- `comparisons = wins + losses` (maintained in the vote transaction).
- only `active = TRUE` rows are eligible for matchups and the leaderboard.

**Indexes**
- `UNIQUE(wikidata_id)` — dedup/upsert key.
- `INDEX((rating - 2 * rd) DESC) WHERE active` — leaderboard query (conservative-rating order, [05](05-ranking.md)).
- `INDEX(comparisons) WHERE active` — coverage-biased pairing ([05](05-ranking.md)).
- `GIN(available_langs)` — fast "has language L" membership (R9).

### `subject_translations`

Per-language display content, **lazily cached** from the Wikipedia summary API.
English is always present; other languages are inserted on first request and
reused thereafter (D7).

| Column | Type | Notes |
|--------|------|-------|
| `subject_id` | BIGINT NOT NULL | FK → `subjects.id` (ON DELETE CASCADE) |
| `lang` | TEXT NOT NULL | Wikipedia language code (`en`, `fr`, …) |
| `name` | TEXT NOT NULL | localized display name |
| `description` | TEXT | localized one-liner (summary `description`/`extract`) |
| `extract` | TEXT | the summary **lead paragraph**, shown inline on the matchup card so a visitor can form an opinion without leaving (0004); nullable until backfilled |
| `image_url` | TEXT | thumbnail from that edition; nullable → placeholder |
| `wikipedia_url` | TEXT NOT NULL | page URL **in this language** (R-I2) |
| `fetched_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | cache freshness |
| | | **PRIMARY KEY (`subject_id`, `lang`)** |

**Invariants**
- `(subject_id, 'en')` exists for every subject (universal fallback).
- a translation's `lang` is always a member of the subject's `available_langs`.
- `wikipedia_url` never empty (R2, per language).

### `votes`

Append-only log of every recorded preference. The audit trail and recompute
source.

| Column | Type | Notes |
|--------|------|-------|
| `id` | BIGINT IDENTITY PK | |
| `session_id` | TEXT NOT NULL | FK → `sessions.id`; which browser voted |
| `winner_id` | BIGINT NOT NULL | FK → `subjects.id`; preferred subject |
| `loser_id` | BIGINT NOT NULL | FK → `subjects.id`; the other subject |
| `winner_rating_before` | DOUBLE PRECISION NOT NULL | audit; winner's `rating` before the update |
| `loser_rating_before` | DOUBLE PRECISION NOT NULL | audit; loser's `rating` before the update |
| `winner_rd_before` | DOUBLE PRECISION | audit; winner's `rd` before (NULL for pre-Glicko-2 rows) |
| `loser_rd_before` | DOUBLE PRECISION | audit; loser's `rd` before (NULL for pre-Glicko-2 rows) |
| `winner_vol_before` | DOUBLE PRECISION | audit; winner's `volatility` before (NULL for pre-Glicko-2 rows) |
| `loser_vol_before` | DOUBLE PRECISION | audit; loser's `volatility` before (NULL for pre-Glicko-2 rows) |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |

**Invariants**: `winner_id <> loser_id`; both reference `active` subjects at vote
time; never updated/deleted in v1.
**Indexes**: `INDEX(session_id)`, `INDEX(created_at)`.

> Note: a vote is what mints the **24h access token** (R10). The token itself is
> not persisted — it is a signed value verified statelessly ([04](04-api.md)).

### `sessions`

Anonymous, non-identifying browser token. Counts a visitor's contributions for
the UI ("you've voted N times"), aids pairing variety, and carries the anonymous
**human-verified** status (R12). It is **not** the leaderboard gate (that is the
access token) and holds **no** PII.

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | opaque random id (also the `eagora_session` cookie value) |
| `contributions` | INTEGER NOT NULL DEFAULT 0 | votes by this session |
| `human_verified_until` | TIMESTAMPTZ | humanity-check status expiry (R12); NULL = not verified; voting requires `> now()` |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |
| `last_seen_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | updated each request |

> The humanity **challenge** itself is **stateless and signed** (like the access
> token) — no challenge table; only this expiry timestamp is persisted.

### `subject_add_log`

Enforces **one add per access token** (R8.1) and audits user-adds. A row is
written when an add succeeds; the token's random `jti` is the primary key, so a
second add with the same token conflicts and is rejected. Holds **no**
identifier (anonymity preserved).

| Column | Type | Notes |
|--------|------|-------|
| `jti` | TEXT PRIMARY KEY | the access token's random id; claims the single allowance |
| `subject_id` | BIGINT NOT NULL REFERENCES subjects(id) | the subject that was added (audit) |
| `token_exp` | TIMESTAMPTZ NOT NULL | token expiry; rows past this are prunable |
| `created_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |

**Use**: `INSERT … ON CONFLICT (jti) DO NOTHING` claims the allowance atomically;
zero rows affected → `429 add_limit_reached`. Prune `WHERE token_exp < now()`.

### `schema_migrations`

| Column | Type | Notes |
|--------|------|-------|
| `version` | INTEGER PK | migration number |
| `applied_at` | TIMESTAMPTZ NOT NULL DEFAULT now() | |

## DDL (initial migration `0001_init.sql`)

```sql
CREATE TABLE subjects (
  id              BIGINT  GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  wikidata_id     TEXT    NOT NULL UNIQUE,
  canonical_name  TEXT    NOT NULL,
  country         TEXT,
  source          TEXT    NOT NULL DEFAULT 'seed' CHECK (source IN ('seed','user')),
  available_langs TEXT[]  NOT NULL DEFAULT '{}',
  rating          DOUBLE PRECISION NOT NULL DEFAULT 1500,
  rd              DOUBLE PRECISION NOT NULL DEFAULT 350,    -- Glicko-2 rating deviation
  volatility      DOUBLE PRECISION NOT NULL DEFAULT 0.06,   -- Glicko-2 volatility (σ)
  wins            INTEGER NOT NULL DEFAULT 0,
  losses          INTEGER NOT NULL DEFAULT 0,
  comparisons     INTEGER NOT NULL DEFAULT 0,
  active          BOOLEAN NOT NULL DEFAULT TRUE,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_subjects_board    ON subjects ((rating - 2 * rd) DESC) WHERE active;
CREATE INDEX idx_subjects_coverage ON subjects(comparisons) WHERE active;
CREATE INDEX idx_subjects_langs    ON subjects USING GIN (available_langs);

CREATE TABLE subject_translations (
  subject_id    BIGINT NOT NULL REFERENCES subjects(id) ON DELETE CASCADE,
  lang          TEXT   NOT NULL,
  name          TEXT   NOT NULL,
  description   TEXT,
  extract       TEXT,  -- lead paragraph for the card (added in 0004)
  image_url     TEXT,
  wikipedia_url TEXT   NOT NULL,
  fetched_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (subject_id, lang)
);

CREATE TABLE sessions (
  id                    TEXT    PRIMARY KEY,
  contributions         INTEGER NOT NULL DEFAULT 0,
  human_verified_until  TIMESTAMPTZ,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_seen_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE votes (
  id                    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  session_id            TEXT   NOT NULL REFERENCES sessions(id),
  winner_id             BIGINT NOT NULL REFERENCES subjects(id),
  loser_id              BIGINT NOT NULL REFERENCES subjects(id),
  winner_rating_before  DOUBLE PRECISION NOT NULL,
  loser_rating_before   DOUBLE PRECISION NOT NULL,
  winner_rd_before      DOUBLE PRECISION,                   -- Glicko-2 snapshots; NULL for
  loser_rd_before       DOUBLE PRECISION,                   -- rows written before 0002
  winner_vol_before     DOUBLE PRECISION,
  loser_vol_before      DOUBLE PRECISION,
  created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (winner_id <> loser_id)
);
CREATE INDEX idx_votes_session ON votes(session_id);
CREATE INDEX idx_votes_created ON votes(created_at);

CREATE TABLE subject_add_log (
  jti        TEXT   PRIMARY KEY,
  subject_id BIGINT NOT NULL REFERENCES subjects(id),
  token_exp  TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE schema_migrations (
  version    INTEGER PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

> PostgreSQL enforces foreign keys by default and uses MVCC, so concurrent
> readers never block on the vote transaction. Partial indexes (`WHERE active`)
> keep the hot leaderboard/coverage queries lean; the GIN index answers
> "subject has language L" for the R9 fallback decision.

## Localization read path (R9)

For a matchup of subjects A, B and visitor language `L`:
1. `display = L` if `L = ANY(A.available_langs) AND L = ANY(B.available_langs)`,
   else `display = 'en'` (the fallback; `fallbackApplied = (display != L)`).
2. For each subject, read `subject_translations(subject_id, display)`. On a cache
   miss, fetch the `{display}.wikipedia.org` summary, insert the row, then serve.
3. Return localized `name`/`description`/`image_url`/`wikipedia_url` + `display`.

The leaderboard localizes the same way using the request's resolved language
(falling back to `en` per entry if a subject lacks that language).

## Derived data & integrity

- **Leaderboard** = `SELECT ... FROM subjects WHERE active ORDER BY
  (rating - 2 * rd) DESC, rd ASC, canonical_name ASC` (conservative-rating order,
  deterministic tie-break), joined to `subject_translations` for the display
  language.
- **Total votes** = `COUNT(*) FROM votes`.
- **Public stats** ([04](04-api.md) §GET /api/stats) are all derived on read —
  no new tables. All-time totals are counts over `votes`, `sessions`, and
  `subjects`; the daily time series buckets `votes`/`sessions`/`subjects` by
  `(created_at AT TIME ZONE 'UTC')::date`, gap-filled with `generate_series`.
  "Visitors" = new `sessions` per day (`sessions.created_at`); "voters" =
  `COUNT(DISTINCT session_id)`. Every figure is an aggregate **count** — no
  per-visitor row, geography, or PII is exposed (the IP is never stored).
- **Recompute path**: because `votes` is append-only and snapshots the full
  pre-vote state (`*_rating_before`, `*_rd_before`, `*_vol_before`), ratings can
  be rebuilt by replaying votes in `created_at` order if the Glicko-2 parameters
  change (see [05](05-ranking.md)).

## Data lifecycle

- **Seed**: `subjects` + English translations populated once from Wikidata +
  Wikipedia ([06](06-wikipedia-ingestion.md)); re-seed upserts on `wikidata_id`
  and **never** resets ratings/vote history.
- **User adds** (R8): insert with `source='user'`; same QID dedupe; new subjects
  start at rating 1500.
- **Translations**: grow lazily as languages are requested; safe to evict/refresh
  by `fetched_at` (a future cache policy; not required for v1).
- **Sessions**: created lazily; prunable by `last_seen_at` age later.
- **Votes**: retained indefinitely (small, valuable audit trail).
