# 04 — API Contract

REST/JSON over HTTP between the Vue SPA and the Go backend. All endpoints are
under `/api`. JSON in, JSON out, UTF-8.

## Conventions

- **Base path**: `/api`.
- **Content type**: `application/json` for request and response bodies.
- **Cookies** (the client uses `fetch(..., { credentials: 'include' })`):
  - `eagora_session` — anonymous **contribution counter** (UI only). Minted if
    absent. Non-identifying. **Not** the gate.
  - `eagora_lb_access` — the **24h access token** (R10), minted by voting; the
    leaderboard gate. Stateless, signed, carries **no** identifier.
  Both are `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` over HTTPS. Neither is
  authentication (R3) — one counts, one authorizes a read for 24h.
- **Language**: endpoints that return subject content read the visitor language
  from the `Accept-Language` header, overridable with a `?lang=<code>` query
  param. The server maps it to a Wikipedia language code (primary subtag;
  `pt-BR`→`pt`) and applies the R9 rule.
- **Timestamps**: ISO-8601 UTC (e.g. `2026-05-31T12:00:00Z`).
- **IDs**: integers for subjects/votes; `wikidataId` (e.g. `Q567`) is the stable
  external identity; the session id is opaque and never exposed in bodies.

### Error envelope

Non-2xx responses share one shape:

```json
{ "error": "access_required", "message": "Vote once to unlock the rankings for 24 hours." }
```

| `error` code | HTTP | Meaning |
|--------------|------|---------|
| `access_required` | 403 | Leaderboard requested with no access token (R4/R10). |
| `access_expired` | 403 | Access token present but older than 24h (R10). |
| `human_check_required` | 403 | Vote attempted by a session that isn't human-verified (R12); the client must pass the humanity check. |
| `invalid_matchup` | 400 | winner/loser missing, equal, inactive, or not valid. |
| `not_a_wikipedia_page` | 422 | Add: input doesn't resolve to a Wikipedia page (R2). |
| `not_a_person` | 422 | Add: resolves, but the entity isn't a human (R8). |
| `already_exists` | 409 | Add: a subject with that Wikidata QID is already in the pool. |
| `add_limit_reached` | 429 | Add: this access token already used its one add (R8.1). |
| `pool_too_small` | 409 | Fewer than 2 active subjects; no matchup possible. |
| `rate_limited` | 429 | Vote/add throughput for this session exceeded the limit (R11, on by default); includes a `Retry-After` header. |
| `not_found` | 404 | Unknown route or resource. |
| `internal` | 500 | Unexpected server error. |

## Resource shapes

### `Subject` (public, localized projection)

Rendered in the request's **display language** (matchup) or per-entry language
(leaderboard).

```json
{
  "id": 42,
  "wikidataId": "Q567",
  "name": "Angela Merkel",
  "description": "Chancellor of Germany 2005–2021",
  "extract": "Angela Dorothea Merkel is a German former politician who served as Chancellor of Germany from 2005 to 2021…",
  "imageUrl": "https://upload.wikimedia.org/.../Angela_Merkel.jpg",
  "wikipediaUrl": "https://de.wikipedia.org/wiki/Angela_Merkel"
}
```

> The matchup projection deliberately **omits `rating`/`wins`/`losses`** so the
> visitor isn't biased before choosing. `extract` is the Wikipedia lead paragraph
> (shown inline on the card; omitted when not yet cached), and `wikipediaUrl`
> always points to the page in the language shown (R-I2).

### `LeaderboardEntry`

```json
{
  "rank": 1,
  "subject": { "...Subject..." : "" },
  "rating": 1624.8,
  "ratingDeviation": 62.4,
  "wins": 51,
  "losses": 19,
  "comparisons": 70,
  "lang": "en"
}
```
`rating` is the Glicko-2 rating; `ratingDeviation` (RD) is its uncertainty — a
high value (≳110) means the entry is still **provisional** and the client may
flag it. `lang` is the language this entry was localized to (the requested
language, or `en` if that subject lacks it).

## Endpoints

### `GET /api/matchup`

Return two distinct, active subjects to compare, localized per R9. Mints a
session cookie if none.

**Query**: `lang` (optional override).

**200**
```json
{
  "a": { "...Subject..." : "" },
  "b": { "...Subject..." : "" },
  "displayLang": "fr",
  "fallbackApplied": false
}
```
- `displayLang` — the language **both** cards are shown in.
- `fallbackApplied` — `true` when the visitor's language was dropped to English
  because one subject lacked it (drives the "Shown in English…" note, R9).

**409** `pool_too_small` if fewer than two active subjects exist.

Read-only and safe to call repeatedly (e.g. "skip"). On a translation cache
miss for `displayLang`, the server fetches+caches it before responding.

### `GET /api/human/challenge`

Issue an anonymous humanity challenge (R12). Mints a session cookie if none.

**200**
```json
{
  "challengeId": "<signed: promptId, passCondition, nonce, exp — opaque to client>",
  "prompt": "I fully trust that all world leaders serve humanity's interest before their own.",
  "kind": "oath",
  "options": [
    { "id": "affirm",  "label": "I swear it" },
    { "id": "dissent", "label": "I won't swear to that" }
  ]
}
```
**Click-only** — the visitor never types. Option order is randomized; the correct
(pass) option is **never** revealed. `kind` is `oath` (pass = dissent) or
occasionally `control` (a sincere statement where pass = agree), so an "always
dissent" policy fails. Default provider is `dissent`; with `turnstile`/`pow` the
shape carries that provider's widget data instead.

### `POST /api/human/verify`

Submit an answer to a challenge.

**Request** — `timing` is a small client-collected interaction summary (no
keystrokes; just decide-latency in ms, an instant-click flag, coarse pointer
cadence) used as a **soft** behavioral signal.
```json
{ "challengeId": "<from the challenge>", "answer": "dissent",
  "timing": { "decideMs": 2400, "instant": false, "pointerMoves": 7 } }
```

**200 (pass)** — sets/updates `sessions.human_verified_until`
```json
{ "verified": true, "humanVerifiedUntil": "2026-06-01T12:00:00Z" }
```
**200 (fail)** — wrong choice; client retries with the returned fresh challenge
```json
{ "verified": false, "reason": "try_again", "challengeId": "<fresh>", "prompt": "…", "options": [ … ] }
```
Validates the signed `challengeId` (signature + `exp` + nonce), then checks the
pass condition and that `timing` isn't bot-flagged. `timing` **never hard-fails
on its own** (accessibility) and is **not stored** (privacy). Repeated calls fall
under the rate limit (R11); each challenge is single-use within its short `exp`.

### `POST /api/votes`

Record a preference, update ratings, and **mint a 24h access token when none is
currently valid** (R10; the window is fixed, not rolling). This is the
contribution that satisfies the gate (R4). Requires a **human-verified** session
(R12).

**Request**
```json
{ "winnerId": 42, "loserId": 17 }
```

**Preconditions**: the session must be **human-verified** (R12) — else `403
human_check_required` (the client then runs the humanity check and retries).
**Validation**: both ids present, integers, `winnerId <> loserId`, both `active`.

**200** — sets `Set-Cookie: eagora_lb_access=<signed token>; Max-Age=86400; …`
```json
{
  "contributions": 4,
  "accessTokenExpiresAt": "2026-06-01T12:00:00Z",
  "a": { "...updated winner Subject (rating/ratingDeviation may be included here)..." : "" },
  "b": { "...updated loser Subject..." : "" }
}
```
`accessTokenExpiresAt` lets the UI show the countdown; the cookie carries the
actual gate credential. `contributions` is this session's running total (UI).

**403** `human_check_required` (not human-verified — R12). **400**
`invalid_matchup`. **429** `rate_limited` (per-session limit, on by default —
R11) with a `Retry-After` header; the vote is not recorded.

Also sets the cookie only when no valid token is held; while a token is valid the
`Set-Cookie` is omitted (or re-sends the same value) and `accessTokenExpiresAt`
reflects the existing fixed window (R10).

**Idempotency**: not idempotent — each call is a distinct contribution. The first
vote with no valid token starts the 24h window; later votes within it don't
extend it. The client must not auto-retry a request whose outcome is unknown
without user intent (avoid double counting); a single retry on a hard network
error is acceptable.

### `GET /api/leaderboard`

Ranked standings. **Gated by the access token (R4 + R10).**

**Query**: `lang` (optional), `limit` (default 100, max 500), `offset` (default 0).

**403** when the `eagora_lb_access` cookie is **missing/invalid** →
`access_required`; when **expired** → `access_expired`:
```json
{ "error": "access_expired", "message": "Your 24-hour access has expired — vote again to unlock." }
```

**200**
```json
{
  "totalVotes": 12840,
  "limit": 100, "offset": 0, "count": 100,
  "entries": [
    { "rank": 1, "subject": { }, "rating": 1624.8, "ratingDeviation": 62.4, "wins": 51, "losses": 19, "comparisons": 70, "lang": "en" }
  ]
}
```
Ordering: `(rating - 2*rd) DESC, rd ASC, canonical_name ASC` (conservative Glicko-2
rating; deterministic tie-break — see [05](05-ranking.md) §Leaderboard ordering).
Each entry localizes to the requested language, falling back to `en` per subject.

### `POST /api/subjects`

Add a new subject to the pool (R8). **Gated and rate-limited** (R8.1): requires a
valid `eagora_lb_access` token, and **each token may add only once**. Accepts
**one** of: a Wikipedia `url`, a free-text `query` (resolve via search first), or
a `wikidataId`.

**Request**
```json
{ "url": "https://en.wikipedia.org/wiki/Jacinda_Ardern" }
```

**Pipeline**: verify token → **claim the token's single add allowance** (atomic
`jti` insert; already used → `429`) → resolve → Wikidata QID → assert page exists
(R2) → assert *instance of: human* (R8) → dedupe by QID → fetch `available_langs`
+ English summary → insert (`source='user'`, rating 1500).

**201**
```json
{ "subject": { "...Subject (English projection)..." : "" } }
```

**403** `access_required` / `access_expired` (no valid token — vote first).
**429** `add_limit_reached` (this token already added someone — wait for a new
24h window). **422** `not_a_wikipedia_page` / `not_a_person`; **409**
`already_exists` (optionally include the existing subject's `id`).

> Note: a rejected add (4xx/409/422) does **not** consume the allowance — the
> `jti` claim is released/rolled back unless the insert succeeds.

### `GET /api/subjects/search`

Autocomplete to help a visitor pick the right person before adding. Proxies
Wikipedia search.

**Query**: `q` (required), `lang` (optional).

**200**
```json
{
  "results": [
    { "title": "Jacinda Ardern", "description": "Prime Minister of New Zealand 2017–2023",
      "imageUrl": "https://...", "wikipediaUrl": "https://en.wikipedia.org/wiki/Jacinda_Ardern" }
  ]
}
```
Selecting a result hands its `wikipediaUrl` to `POST /api/subjects`. (QID and the
human-check are resolved server-side at add time.)

### `GET /api/me`

Lightweight state for the client: drives the leaderboard lock/unlock UI and the
countdown. Mints a session cookie if none. Reads (but never mints) the access
token.

**200**
```json
{
  "contributions": 4,
  "hasAccess": true,
  "accessExpiresAt": "2026-06-01T12:00:00Z",
  "canAdd": true,
  "humanVerified": true,
  "humanVerifiedUntil": "2026-06-01T12:00:00Z"
}
```
`hasAccess` is `false` (and `accessExpiresAt` null) when there is no valid token.
`canAdd` is `true` only when a valid token is held **and** its one add allowance
is unused (R8.1). `humanVerified` is `true` while `humanVerifiedUntil > now`
(R12); when `false`, the client should expect the humanity check on the next
vote attempt.

### `GET /api/stats`

Public transparency dashboard data. **Ungated** (no token, no session minted):
the figures are aggregate counts over **anonymous** data and reveal nothing about
any individual visitor (no IP, geography, account, or per-visitor row is ever
stored or returned — see §Abuse & integrity).

**Query**: `days` (trailing UTC-day window; default `30`, min `1`, max `365`).

**200**
```json
{
  "generatedAt": "2026-05-31T12:00:00Z",
  "days": 30,
  "totals": {
    "votes": 12840,
    "voters": 1932,
    "visitors": 2517,
    "subjects": 391,
    "userContributed": 28
  },
  "daily": [
    { "date": "2026-05-02", "votes": 412, "voters": 73, "visitors": 88, "added": 1 }
  ]
}
```
- `totals` are **all-time**: `votes` (preferences recorded), `voters` (distinct
  anonymous sessions that ever voted), `visitors` (anonymous sessions ever
  created ≈ unique browsers), `subjects` (active people in the pool), and
  `userContributed` (subjects with `source='user'`).
- `daily` is a **gap-filled** series of exactly `days` UTC-day buckets (zeros
  where there was no activity), each with that day's `votes`, distinct `voters`,
  new `visitors` (sessions first seen that day), and people `added`.

> "Visitors" is derived from `sessions.created_at` — the first time a browser is
> seen — so it counts **new** anonymous browsers per day, not raw page views
> (which e-agora deliberately does not log). Every value is a `COUNT`; there is
> no per-visitor data to expose.

### `GET /api/healthz`

Liveness/readiness. **200** `{ "status": "ok", "subjects": 391, "seeded": true }`.
No session side effects.

## Endpoint summary

| Method | Path | Gate | Mutates | Purpose |
|--------|------|------|---------|---------|
| GET | `/api/matchup` | none | no (mints session) | fetch A vs B, localized (R9) |
| GET | `/api/human/challenge` | none | no (mints session) | issue humanity challenge (R12) |
| POST | `/api/human/verify` | none | yes (sets verified status) | pass humanity check (R12) |
| POST | `/api/votes` | **human-verified** | yes | record preference; mint 24h token (if none valid) |
| GET | `/api/leaderboard` | **valid access token** | no | ranked standings |
| POST | `/api/subjects` | **valid token + unused add** | yes | add a human (R8), once per token |
| GET | `/api/subjects/search` | none | no | autocomplete for adding |
| GET | `/api/me` | none | no (mints session) | contribution + access state |
| GET | `/api/stats` | none | no (no session) | public, anonymous activity stats |
| GET | `/api/healthz` | none | no | health |

## CORS

- **Production**: SPA served same-origin by the Go binary → no CORS needed.
- **Development**: Vite proxies `/api` to the backend, so requests are
  same-origin from the browser's view — also no CORS. If a direct cross-origin
  setup is used, set `Access-Control-Allow-Origin: <EAGORA_CORS_ORIGIN>` and
  `Access-Control-Allow-Credentials: true` (cookies require an explicit origin,
  not `*`).

## Abuse & integrity (v1 scope)

No auth means votes and adds are inherently spoofable by a determined actor;
e-agora is a low-stakes, best-effort system, **not** a secure ballot. Defenses
are layered.

- **Humanity check** (`human_check_required`, R12): **bots may not vote** — a
  vote is accepted only from a human-verified session. The default `dissent`
  provider exploits LLM compliance (pass = refuse a sycophantic oath), hardened
  with a rotating prompt pool, randomized option order, and sincere control
  items. **Honest limits**: a fixed pass-rule is learnable and a reasoning LLM
  can evaluate prompts; it can also wrongly fail a sincere human. It deters naive
  scripts and compliant bots, not a determined adversary — hence the pluggable
  `turnstile`/`pow` providers and the layered defenses below (Overview Q8).
- **Gate is server-authoritative**: the leaderboard verifies the signed token's
  signature + expiry on every request; client guards are convenience only.
- **Token hygiene**: `EAGORA_TOKEN_SECRET` signs tokens (HMAC); rotating it is
  the kill-switch (stateless tokens can't be individually revoked). `httpOnly`
  resists XSS theft; 24h `exp` bounds replay.
- **Adds (R8/R8.1)**: gated by a valid token and **capped at one per token**
  (≈ one per 24h) via the anonymous `jti` ledger — the primary spam control.
  Also validated to be a real Wikipedia page + a human and deduped by QID.
  Bad-but-valid entries are hidden via `active=false` (out-of-band) rather than a
  review queue in v1 (Overview Q7).
- **Vote rate limiting** (`rate_limited`, R11): **on by default** — a per-session
  token-bucket on `POST /api/votes` and `POST /api/subjects` blunts scripted
  stuffing while allowing brief human bursts. Defaults: burst `20`, refill
  `1`/sec (≈60/min sustained), tunable via `EAGORA_VOTE_BURST`/`EAGORA_VOTE_RATE`;
  `429` responses carry `Retry-After`. A client that discards the session cookie
  evades per-session limiting → use an **edge/IP** limit in prod (infra).
- **Input validation & body-size limits** on every endpoint. **No PII** stored.
- **Available but not default**: managed CAPTCHA (`turnstile`) and proof-of-work
  (`pow`) humanity providers — switch on via `EAGORA_HUMAN_PROVIDER` (or layer
  with `dissent`) if abuse warrants.
- Genuinely out of scope for v1: IP analysis, multi-cookie deduplication,
  edge/IP rate limiting (infra). Revisit only if abuse is observed.
