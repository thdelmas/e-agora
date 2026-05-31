# 07 — Roadmap

Build order, milestones, and acceptance criteria. Each milestone is shippable and
verifiable on its own. Guiding principle: **get the core loop
(land → vote → leaderboard) working end-to-end early, then deepen** with i18n and
user-adds.

## Milestones

### M0 — Specs & scaffolding ✅ (done)
- [x] Write `docs/` (this set).
- [x] `.gitignore`, repo skeleton (`backend/`, `frontend/`).
- [x] `docker-compose.yml` for local PostgreSQL.
- **Done when**: specs reviewed; empty modules build (`go build`, `npm run dev`).
  *Met: `go build ./...` + `go vet` clean, `go test ./...` green (Elo), `npm run
  build` succeeds, server boots and `GET /api/healthz` → 200.*

### M1 — Backend skeleton + DB ✅ (done)
- [x] Go module, `cmd/server/main.go`, chi router, `slog`, graceful shutdown.
- [x] `pgxpool` from `DATABASE_URL`; fail loudly if unreachable.
- [x] Embedded migration runner; `0001_init.sql` (subjects, subject_translations,
      votes, sessions, subject_add_log, schema_migrations).
- [x] `GET /api/healthz` → subject count + seeded flag.
- **Done when**: server boots, migrations apply to a fresh DB, `healthz` is 200.
  *Met: boot applies `0001_init.sql` (6 tables + partial/GIN indexes), reboot
  applies 0 (idempotent), `healthz` → `200 {subjects:0, seeded:false}`, and an
  unreachable DB exits non-zero with a clear hint. The runner owns
  `schema_migrations` (bootstrapped before reading), so it's not in `0001`.*

### M2 — Ingestion (Wikidata → Wikipedia)
- [ ] `internal/ingest`: Wikidata SPARQL (UN leaders), `EntityData` (P31 human
      check, sitelinks→`available_langs`, labels), Wikipedia summary client
      (User-Agent, retry, skip rules).
- [ ] `backend/data/un_leaders.json` snapshot (193 members + 2 observers, HoS+HoG,
      deduped) + optional `seed_extra.json`.
- [ ] Seed-on-startup honoring `EAGORA_SEED` (`auto`/`off`/`force`); English
      translation cached per subject; offline fallback.
- **Done when**: a fresh DB seeds the pool; every subject is a human with `'en' ∈
  available_langs` and a non-empty English `wikipedia_url`; non-humans/missing
  pages are skipped+logged; re-`force` preserves ratings.

### M3 — Matchup + i18n + voting + Elo + token + humanity check ✅ (done)
- [x] `internal/ranking` Elo (pure) **with unit tests** (±16, conservation,
      monotonicity, upset magnitude). *(Superseded by M7 — the engine is now
      Glicko-2; see below.)*
- [x] Matchup pair selection — uniform random in `store.RandomPair`
      (`ORDER BY random()`). *(Superseded by M7 — now coverage-biased; see below.)*
- [x] `internal/lang` (`Accept-Language`/`?lang=` → Wikipedia code) + R9
      resolution; **lazy translation fetch+cache** on miss (`SitelinkTitle`).
- [x] `internal/token` mint/verify stateless HMAC token (`EAGORA_TOKEN_SECRET`,
      fixed 24h, random `jti`) **with unit tests** (valid, tampered, expired).
- [x] Session middleware (mint/read `eagora_session`).
- [x] `internal/ratelimit` per-session token-bucket (R11, on by default) **with
      unit tests** + middleware on `POST /api/votes`.
- [x] `internal/human` (R12): signed-challenge mint/verify, `dissent` provider
      (**click-only**), `humanity_prompts.json` pool (oaths + control items),
      and a **soft interaction-timing** signal that never hard-fails (a11y)
      **with unit tests**; `GET /api/human/challenge`, `POST /api/human/verify`;
      `human_verified_until` gate on `POST /api/votes`.
- [x] `GET /api/matchup` (localized, `displayLang`/`fallbackApplied`),
      `POST /api/votes` (transactional Elo + mint `eagora_lb_access` **only when
      none valid** — fixed window).
- **Done when**: voting moves ratings correctly under concurrency; matchup
  honors R9 (both-or-English); an un-verified vote → `403 human_check_required`,
  and passing the dissent check (refusing the oath) then lets the vote through;
  a first vote returns `accessTokenExpiresAt` and sets the token cookie; extra
  votes within the window don't slide its expiry; exceeding the per-session limit
  returns `429 rate_limited` + `Retry-After`.
  *Met: verified live against the seeded pool — matchup → 403 (no human) →
  dissent challenge → verify → vote → `1516/1484` Elo (±16), token + session
  cookies set, audit row written. `go test ./...` green across `ranking`,
  `token`, `ratelimit`, `lang`, `human`, `ingest`.*

### M4 — Token gate + leaderboard + add-a-subject ✅ (done)
- [x] `GET /api/me` (contributions + `hasAccess`/`accessExpiresAt` + `canAdd` +
      `humanVerified`/`humanVerifiedUntil`).
- [x] `GET /api/leaderboard` → `403 access_required`/`access_expired` without a
      valid token, else ranked, localized entries (+ `totalVotes`).
- [x] `internal/subjects` + `POST /api/subjects` — **token-gated, one add per
      token** (atomic `jti` claim in `subject_add_log`); resolve URL→QID
      (pageprops), human-check, dedupe, ingest — and `GET /api/subjects/search`
      (Wikipedia REST autocomplete).
- [x] `/api/me` exposes `canAdd` (valid token + unused allowance).
- **Done when**: no/expired token → 403; after one vote → 200 ordered entries;
  adding a human URL (with a token) inserts it and it appears in later matchups;
  a **second add on the same token → 429 `add_limit_reached`**, and a rejected
  add doesn't consume the allowance; non-person / duplicate / non-page inputs
  return the right 4xx codes.
  *Met: verified live — `leaderboard` 403 (no token) → vote → 200 with the +16
  winner ranked #1; `POST /api/subjects` of a Wikipedia URL resolved → inserted
  (201), a 2nd add on the same token → 429, `/api/me canAdd` flipped true→false;
  search returns suggestions. `subjects` + `ingest` unit tests green.*

### M5 — Frontend ✅ (code-complete; in-browser walkthrough pending)
- [x] Vue 3 + Vite + Router (`/`, `/leaderboard`, add modal via App), `api/client.js`
      (`credentials: 'include'`, `timing` on verify), Vite dev proxy + shared
      reactive `store.js` (`me`, `refreshMe`, `applyVote`).
- [x] `MatchupView` + `PoliticianCard` (localized content, Wikipedia link, vote +
      skip, keyboard ←/→/S, toast) + `LanguageNote` (R9 fallback).
- [x] `AddSubjectModal` (paste URL / search-autocomplete / confirm) with inline
      eligibility errors (not_a_person / already_exists / add_limit_reached / …).
- [x] `HumanityCheckModal` (S4): shown on `403 human_check_required`; refuse the
      oath → verify (with interaction timing) → auto-retry the pending vote.
- [x] `AccessBanner` live 24h countdown + lock/unlock from the store; route
      guard mirroring the token gate.
- [x] `LeaderboardView` + `LeaderboardRow`; total-votes stat; "keep voting"; nav
      with contribution count + gated "Add someone" / "Rankings 🔒".
- [x] Persistent neutrality disclaimer (`SiteFooter`).
- **Done when**: a human can complete J1 (humanity check → vote → unlock), J5
  (add a subject), J6 (English-fallback matchup), and J7 (pass the humanity
  check) in a browser.
  *Code-complete: `npm run build` green (38 modules); all components compile
  against the M3/M4 API verified live. The final in-browser J1/J5/J6/J7
  click-through is the one step that needs a human at a browser — run `make dev`
  (free :8080 first) and walk it.*

### M6 — Polish & hardening ✅ (done)
- [x] Responsive/mobile (flex cards + nav media query); loading/empty/error
      states; a11y — alt text, modal Esc-to-close, `aria-live` banner/toast.
- [x] Body-size limits (`MaxBytesReader` on all mutating endpoints) + input
      validation; per-session rate limit applied to `POST /api/subjects` too;
      edge/IP limit documented for prod ([04](04-api.md) §Abuse).
- [x] Production build: `EAGORA_STATIC_DIR` makes Go serve the built SPA
      same-origin (with SPA fallback); `Dockerfile.prod` bundles SPA + backend
      into one image (`make prod-build`).
- [x] README verified from scratch (dev + prod); `EAGORA_TOKEN_SECRET` **required**
      — the server refuses to boot without it.
- **Done when**: `J1`–`J6` all behave per [functional spec](01-functional-spec.md).
  *Met (server-side): one binary serves `/` + client routes (SPA fallback) +
  real assets + the gated API (`/api/leaderboard` → 403 without a token), and
  boot fails fast on a missing secret — all verified live. The full in-browser
  J1–J6 walkthrough still wants a human at a browser (`make dev`).*

### M7 — Glicko-2 rating engine ✅ (done)
Replace Elo with Glicko-2 so ratings carry an explicit uncertainty: visitor-added
subjects converge fast and the board ranks conservatively until a rating is proven.
- [x] `internal/ranking` rewritten to Glicko-2 (pure, value-typed `Rating{R,RD,Vol}`;
      each vote = a one-game rating period; Illinois-algorithm volatility solver),
      **with unit tests** validated against Glickman's published worked example.
- [x] Migration `0002_glicko2.sql`: `subjects.rd`/`volatility` columns, conservative
      board index `((rating - 2*rd) DESC)`, and `votes` pre-vote snapshots
      (`*_rd_before`, `*_vol_before`); existing subjects keep their rating with an
      evidence-scaled starting RD.
- [x] `RecordVote` reads/writes the full `(rating, rd, volatility)` state; leaderboard
      orders by conservative rating; API exposes `ratingDeviation`; the board UI marks
      high-RD entries **provisional**.
- [x] **Coverage-biased pairing** — `store.RandomPair` weights selection by
      `1/(comparisons+1)` via Efraimidis–Spirakis (`power(random(), comparisons+1)`),
      the supply side of conservative ordering: unproven subjects get shown (and their
      `RD` tightened) fast, so the board doesn't ossify to the seed pool.
- **Done when**: ranking tests green (incl. the Glickman anchor); voting moves
  `R`/`RD`/`σ` correctly under concurrency; the board sorts by `rating − 2·RD`;
  new subjects are oversampled until their `RD` shrinks.

### M8 — Public stats dashboard ✅ (done)
Open, anonymous transparency page so anyone (no vote required) can see the agora's
activity, while honoring the anonymity promise (R3 / no PII).
- [x] `store.Stats(days)` — all-time totals (votes, distinct voters, visitors,
      pool size, user-added) + a gap-filled daily series
      (votes/voters/visitors/added), bucketed by UTC day via `generate_series`.
      Derived on read; **no new tables** (docs/03-data-model.md §Derived data).
- [x] `GET /api/stats` — **public/ungated**, mints **no** session; `days` window
      (default 30, 1–365) ([04](04-api.md) §GET /api/stats).
- [x] Frontend `/stats` route (ungated) + `StatsView` with headline stat cards,
      a 7/30/90-day picker, and dependency-free SVG charts (`TimeSeriesChart`:
      area + bars, responsive, `aria-label` + per-point `<title>`); nav link.
- **Done when**: `/api/stats` returns aggregate counts with no per-visitor data;
  "Votes over time" and "Visitors over time" render with other privacy-safe
  metrics; the page is reachable without an access token.

## Definition of done (v1)

All requirements demonstrably met:
- **R1/R1.1/R2** — pool seeds with every UN member + observer state's HoS & HoG,
  deduped; every card links to a real Wikipedia page; ingestion skips
  unresolvable/non-human entries.
- **R3** — no auth anywhere; only an anonymous session cookie + an identifier-free
  access token exist.
- **R4/R10** — leaderboard returns `403` without a valid token; voting mints a
  24h token; expiry re-locks. Verified by test and by hand.
- **R5** — land → A vs B → vote → leaderboard works in the browser.
- **R6** — leaderboard order is driven purely by accumulated votes (Glicko-2
  conservative rating, `rating − 2·RD`).
- **R8/R8.1** — a visitor with a valid token can add any human with a Wikipedia
  page; it enters matchups; adds are capped at one per token (≈ one per 24h).
- **R9** — matchups render in the visitor's language when both subjects have it,
  else English, never mixed.
- **R11** — voting is rate-limited per session (on by default); exceeding it
  returns `429 rate_limited` + `Retry-After`.
- **R12** — bots can't vote: an un-verified session is refused
  (`403 human_check_required`) until it passes the anonymous humanity check;
  passing grants a time-boxed human-verified status.

## Testing strategy

| Layer | Tests |
|-------|-------|
| `ranking` | unit (pure Glicko-2 properties: direction, RD shrinks with evidence, non-conservation, upset magnitude) + Glickman's published worked example as a regression anchor |
| `token` | unit (valid / tampered signature / expired / wrong secret); fixed-window mint-if-none; one-add-per-`jti` ledger claim incl. concurrent-claim race |
| `ratelimit` | unit (burst admitted, refill over time, empty→reject, per-key isolation, idle eviction) |
| `human` | unit (signed challenge round-trips; tampered/expired/replayed → reject; dissent pass / affirm fail; control-item inversion; instant-click flagged but slow/assistive timing **not** blocked; per-provider) |
| `lang` | unit (Accept-Language parsing; R9 both-or-English decision) |
| `store` | integration vs disposable Postgres (testcontainers / CI service) |
| `ingest` | parse tests on captured Wikidata/summary fixtures; human-check & skip rules |
| `http` | matchup shape + `displayLang`/`fallback`; vote deltas + token issuance; **gate 403/200**; rate-limit **429 + Retry-After** on votes; add-subject success + **403 (no token) / 429 (second add)** / 422 / 409; validation errors |
| frontend | component smoke + manual E2E of J1/J5/J6 (optional Playwright later) |

## Risks & mitigations

| Risk | Mitigation |
|------|------------|
| Wikidata/Wikipedia throttle the seed | Proper User-Agent, low sequential rate, retry/backoff; committed `un_leaders.json` snapshot + offline fallback (06). |
| `EAGORA_TOKEN_SECRET` unset/weak in prod | Required at boot in prod; rotating it invalidates all tokens (kill-switch). |
| No-auth vote stuffing / spam adds | Layered: humanity check (R12, bots can't vote) + per-session rate limit (R11) + adds gated **one per token** (R8.1) via the `jti` ledger; human+page+QID checks; `active=false` hide switch. Determined attackers out of scope for v1; server gates authoritative. |
| Humanity check defeated / frustrates humans | Layered signals (dissent + soft interaction-timing); rotating prompt pool + randomized order + sincere control items resist a fixed pass-rule; short window; **pluggable** `turnstile`/`pow` to layer/replace (Overview Q8). Timing **never hard-fails alone** (a11y); honest: not bulletproof vs a reasoning adversary that emulates human timing. |
| Lazy translation fetch adds matchup latency | English pre-cached at seed; popular languages warm quickly; fetch is one summary call, then cached. |
| Subject with no English page (R9 fallback gap) | Require `en` for seed leaders; fallback chain for the rare user-add (Overview Q6). |
| Cold-start rating jitter | Expected; show `comparisons`; consider K-decay later (05). |
| Postgres not running in dev | `docker-compose up db` + README; startup fails loudly with guidance. |

## Sequencing rationale

Backend-first (M1–M4) makes the contract real and testable before any UI. The
core loop is alive and curl-able by end of M4 — including i18n, the 24h token
gate, and user-adds — so the frontend (M5) is a thin client over a proven API;
M6 makes it sturdy.
