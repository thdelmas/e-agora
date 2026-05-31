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

### M3 — Matchup + i18n + voting + Elo + token + humanity check
- [ ] `internal/ranking` Elo (pure) **with unit tests** (±16, conservation,
      monotonicity, upset magnitude).
- [ ] `internal/matchup` pair selection (uniform → coverage-bias).
- [ ] `internal/lang` (`Accept-Language`/`?lang=` → Wikipedia code) + R9
      resolution; **lazy translation fetch+cache** on miss.
- [ ] `internal/token` mint/verify stateless HMAC token (`EAGORA_TOKEN_SECRET`,
      fixed 24h, random `jti`) **with unit tests** (valid, tampered, expired).
- [ ] Session middleware (mint/read `eagora_session`).
- [ ] `internal/ratelimit` per-session token-bucket (R11, on by default) **with
      unit tests** + middleware on `POST /api/votes`.
- [ ] `internal/human` (R12): signed-challenge mint/verify, `dissent` provider
      (**click-only**), `humanity_prompts.json` pool (oaths + control items),
      **never-first-try** (attempt count in the envelope, `EAGORA_HUMAN_MIN_ATTEMPTS`)
      and a **soft interaction-timing** signal that never hard-fails (a11y)
      **with unit tests**; `GET /api/human/challenge`, `POST /api/human/verify`;
      `human_verified_until` gate on `POST /api/votes`.
- [ ] `GET /api/matchup` (localized, `displayLang`/`fallbackApplied`),
      `POST /api/votes` (transactional Elo + mint `eagora_lb_access` **only when
      none valid** — fixed window).
- **Done when**: voting moves ratings correctly under concurrency; matchup
  honors R9 (both-or-English); an un-verified vote → `403 human_check_required`,
  and passing the dissent check (refusing the oath) then lets the vote through;
  a first vote returns `accessTokenExpiresAt` and sets the token cookie; extra
  votes within the window don't slide its expiry; exceeding the per-session limit
  returns `429 rate_limited` + `Retry-After`.

### M4 — Token gate + leaderboard + add-a-subject
- [ ] `GET /api/me` (contributions + `hasAccess`/`accessExpiresAt`).
- [ ] `GET /api/leaderboard` → `403 access_required`/`access_expired` without a
      valid token, else ranked, localized entries.
- [ ] `internal/subjects` + `POST /api/subjects` — **token-gated, one add per
      token** (atomic `jti` claim in `subject_add_log`); resolve→QID,
      human-check, dedupe, ingest — and `GET /api/subjects/search` (autocomplete).
- [ ] `/api/me` exposes `canAdd` (valid token + unused allowance).
- **Done when**: no/expired token → 403; after one vote → 200 ordered entries;
  adding a human URL (with a token) inserts it and it appears in later matchups;
  a **second add on the same token → 429 `add_limit_reached`**, and a rejected
  add doesn't consume the allowance; non-person / duplicate / non-page inputs
  return the right 4xx codes.

### M5 — Frontend
- [ ] Vue 3 + Vite + Router (`/`, `/leaderboard`, add modal), `api/client.js`
      (`credentials: 'include'`), Vite dev proxy.
- [ ] `MatchupView` + `PoliticianCard` (localized content, Wikipedia link, vote +
      skip, keyboard ←/→/S) + `LanguageNote` (R9 fallback).
- [ ] `AddSubjectModal` (paste URL / search-autocomplete / confirm) with inline
      eligibility errors.
- [ ] `HumanityCheckModal` (S4): shown on `403 human_check_required`; refuse the
      oath → verify → auto-retry the pending vote.
- [ ] `AccessBanner` countdown + leaderboard lock/unlock from `/api/me`; route
      guard mirroring the token gate.
- [ ] `LeaderboardView` + `LeaderboardRow`; total-votes stat; "keep voting".
- [ ] Persistent neutrality disclaimer (`SiteFooter`).
- **Done when**: a human can complete J1 (humanity check → vote → unlock), J5
  (add a subject), J6 (English-fallback matchup), and J7 (pass the humanity
  check) in a browser.

### M6 — Polish & hardening
- [ ] Responsive/mobile; loading/empty/error states; a11y (alt text, focus,
      contrast).
- [ ] Body-size limits, input validation; tune rate-limit thresholds; apply the
      limiter to `POST /api/subjects` too; document the edge/IP limit for prod.
- [ ] Production build: Go serves the built SPA same-origin; one binary + Postgres.
- [ ] README run instructions verified from scratch; `EAGORA_TOKEN_SECRET`
      required in prod.
- **Done when**: `J1`–`J6` all behave per [functional spec](01-functional-spec.md).

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
- **R6** — leaderboard order is driven purely by accumulated votes (Elo).
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
| `ranking` | unit (pure Elo properties) |
| `token` | unit (valid / tampered signature / expired / wrong secret); fixed-window mint-if-none; one-add-per-`jti` ledger claim incl. concurrent-claim race |
| `ratelimit` | unit (burst admitted, refill over time, empty→reject, per-key isolation, idle eviction) |
| `human` | unit (signed challenge round-trips; tampered/expired/replayed → reject; dissent pass / affirm fail; control-item inversion; **never-first-try holds attempt 1**; instant-click flagged but slow/assistive timing **not** blocked; per-provider) |
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
| Humanity check defeated / frustrates humans | Layered signals (dissent + soft interaction-timing + never-first-try); rotating prompt pool + randomized order + sincere control items resist a fixed pass-rule; short window; **pluggable** `turnstile`/`pow` to layer/replace (Overview Q8). Timing **never hard-fails alone** (a11y); honest: not bulletproof vs a reasoning adversary that emulates human timing. |
| Lazy translation fetch adds matchup latency | English pre-cached at seed; popular languages warm quickly; fetch is one summary call, then cached. |
| Subject with no English page (R9 fallback gap) | Require `en` for seed leaders; fallback chain for the rare user-add (Overview Q6). |
| Cold-start rating jitter | Expected; show `comparisons`; consider K-decay later (05). |
| Postgres not running in dev | `docker-compose up db` + README; startup fails loudly with guidance. |

## Sequencing rationale

Backend-first (M1–M4) makes the contract real and testable before any UI. The
core loop is alive and curl-able by end of M4 — including i18n, the 24h token
gate, and user-adds — so the frontend (M5) is a thin client over a proven API;
M6 makes it sturdy.
