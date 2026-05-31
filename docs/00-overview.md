# 00 — Overview

## Vision

e-agora turns a hard question — *"who are the world's most preferred political
figures?"* — into a stream of tiny, easy ones: *"between these two, who do you
prefer?"* Each answer is a data point. Aggregated across many visitors and many
matchups, the data points settle into a ranking. No surveys, no star ratings,
no accounts — just repeated binary choices, the same primitive that powers chess
ratings and "this-or-that" ranking games.

The product is deliberately tiny. It does one loop well: **show two, capture a
preference, reflect it in the standings.**

## The five hard requirements (verbatim, then interpreted)

> Politicals people ranking world wide (Wikipedia page mandatory)

- **R1.** The subjects are political figures from around the world.
- **R2.** Every subject is backed by a Wikipedia page. Wikipedia is the
  canonical source for a subject's name, short description, and image. A subject
  with no resolvable Wikipedia page is **not eligible** to appear.

> No user authentification

- **R3.** No authentication of any kind. No accounts, login, email, or
  passwords. A visitor is anonymous.

> Data contribution mandatory before accessing data

- **R4.** A visitor must **contribute at least one vote** before they may
  **read** any ranking data. The leaderboard is gated behind contribution.

> Flow is simple, user lands we ask who do you prefer betwen A&B then we show the
> leaderboard, ranking is based on who is prefered

- **R5.** The primary flow is: *land → matchup (A vs B) → vote → leaderboard.*
- **R6.** The ranking is a function of accumulated preferences (who beats whom),
  not of any external metric.

> Name of the project: e-agora

- **R7.** The project is named **e-agora**.

## Clarified requirements (2026-05-31)

Refinements from the product owner, layered onto R1–R7:

- **R1.1 — Seed scope.** The pool ships with **at least the leaders of every
  country recognized by the UN** — specifically each country's **head of state
  *and* head of government** (deduped when one person holds both). The 193 UN
  member states are the baseline; the 2 UN observer states are included too.
- **R8 — User contribution of subjects.** Beyond the seed, **any visitor may add
  a new subject**, provided it resolves to a Wikipedia page. Scope of "subject"
  is widened from politicians to **any human** (Wikidata *instance of: human*) —
  so the pool can grow past politicians, by visitor choice. (R2 still binds:
  every added subject must have a real Wikipedia page.)
- **R8.1 — Adds are gated and rate-limited.** Adding requires a **valid access
  token** (so a visitor must have voted first), and **each token permits exactly
  one add** — i.e. **at most one addition per 24h** (Q7). Adding does **not**
  mint or refresh a token.
- **R9 — Multilingual display.** A matchup is shown in the **visitor's language**
  when **both** subjects have a page in it; if **either** subject lacks that
  language, **both** are shown in **English** (the universal fallback). One
  matchup is never mixed-language.
- **R10 — Time-boxed access (24h).** Contribution no longer unlocks the
  leaderboard forever. **Casting a vote issues an access token with a fixed
  24-hour TTL**; the leaderboard is readable only while a valid token is held.
  The window is **fixed, not rolling**: a vote mints a token only when none is
  currently valid, so "one add per token" (R8.1) cleanly equals "one add per
  24h." When it expires, the next vote starts a fresh 24h window. (This
  supersedes the earlier "one vote unlocks permanently" default — see Q3.)
- **R11 — Vote rate limit.** Voting is rate-limited **per anonymous session, on
  by default**, to blunt scripted vote-stuffing. A token-bucket allows brief
  human bursts but caps sustained throughput; exceeding it returns `429` with a
  `Retry-After`. Thresholds are configurable. (The same limiter guards adds; see
  [04](04-api.md).)
- **R12 — Humans only (humanity check).** **Bots may not vote.** Voting is gated
  by an anonymous, **multi-signal** humanity check; passing grants a time-boxed
  `human-verified` status on the session (Q2: once per window, re-verify on
  expiry). It is **not** authentication (R3) and stores **no** identity. Signals:
  - **Dissent (hard / semantic).** A sycophantic political "loyalty oath" is
    shown; the visitor **passes by refusing to swear it** — judgment a thinking
    human exercises but a compliant LLM bot tends not to. This is the real gate
    and works with any input method.
  - **Interaction timing (soft / behavioral).** No typing is required — the
    visitor only clicks. We look at the natural timing of the interaction (time
    to read the oath and decide; a suspiciously *instant* answer; pointer/touch
    cadence) to flag scripted clicks. **Never hard-fails on its own** — to avoid
    excluding assistive-tech, voice, or switch-device users (accessibility), it
    only lowers confidence; timing is processed ephemerally and **never stored**
    (privacy).
  Threat model & limitations are documented honestly (Q8, [04](04-api.md)
  §Abuse); the provider is **pluggable** so a managed/PoW check can be layered.

## What this is NOT (non-goals)

- Not a poll with a fixed question set — matchups are generated continuously.
- Not an opinion/comment platform — the only input is a binary preference.
- Not authenticated or personalized — there are no user profiles.
- Not a political endorsement — it ranks *stated preferences of visitors*, and
  the UI must frame it that way (see [functional spec](01-functional-spec.md)).
- Not real-time/multiplayer — eventual consistency of the leaderboard is fine.

## Glossary

| Term | Meaning |
|------|---------|
| **Subject** | A person in the pool, backed by a Wikipedia page. Seeded with UN-country leaders (politicians); visitors may add any human (R8). |
| **Matchup** | A pair (A, B) of distinct subjects presented for comparison. |
| **Vote** | A visitor's choice of a winner and a loser within a matchup. |
| **Contribution** | Casting a vote (or adding a subject). A vote is what mints an access token (R4/R10). |
| **Access token** | A short-lived (**24h**) capability that grants leaderboard reads, minted by voting. Carries **no identifier** — anonymous and stateless (R10). |
| **Humanity check** | An anonymous, dissent-based challenge a visitor must pass before voting (R12). Proves "a thinking human," not *who*. |
| **Human-verified** | A time-boxed status on the anonymous session, set by passing the humanity check; required to vote. |
| **Session** | An anonymous, server-issued cookie used only to count a visitor's contributions for UI. **Not** auth, and **separate** from the access token. |
| **Wikidata QID** | The language-independent identity anchor for a subject (e.g. `Q567`), mapping to per-language Wikipedia pages. |
| **Display language** | The language a matchup is rendered in, resolved per R9 (visitor language if both subjects have it, else English). |
| **Rating** | A subject's Glicko-2 rating (with a deviation/uncertainty); the basis of the ranking. |
| **Leaderboard** | Subjects ordered by their conservative rating (`rating − 2·RD`), descending. |

## Design decisions (made here, detailed in later docs)

These defaults were chosen to satisfy the requirements with the least machinery.
They are recorded so they can be challenged.

| ID | Decision | Rationale | Detailed in |
|----|----------|-----------|-------------|
| D1 | **Glicko-2** for ranking | Pairwise model that also tracks each rating's *uncertainty* (RD) and volatility, so visitor-added subjects converge fast and sort conservatively until proven; supersedes Elo. | [05](05-ranking.md) |
| D2 | **PostgreSQL** (via `pgx`/`pgxpool`) | Robust relational store with real concurrency, transactions, and types; the natural fit for consistent rating updates under concurrent voters. Run locally via Docker for dev. | [02](02-architecture.md), [03](03-data-model.md) |
| D3 | **Stateless 24h signed access token** as the gate (R10) | A voter receives a JWS/HMAC-signed token (`{iss, iat, exp, jti}`, **no subject id**) delivered as an `httpOnly`+`SameSite=Lax` cookie, with a **fixed** 24h window (not rolling). Most anonymous (carries no identifier; nothing to correlate) **and** most secure for a low-stakes read capability (unforgeable, XSS-resistant, time-boxed). Decoupled from the session. The random `jti` also enforces one-add-per-token (D9). | [01](01-functional-spec.md), [04](04-api.md) |
| D4 | **Seed-on-startup** ingestion (Wikidata → Wikipedia) | Keeps the pool real (R2) without a separate ETL service; persisted to PostgreSQL so it runs once. | [06](06-wikipedia-ingestion.md) |
| D5 | **Vue 3 + Vite** frontend, **Go + chi** backend | Matches the requested stack; minimal dependencies. | [02](02-architecture.md) |
| D6 | **Wikidata as the enumerator** of UN-country leaders | A SPARQL query yields each UN member/observer state's head of state (P35) + head of government (P6) as QIDs — authoritative, language-independent, and refreshable. | [06](06-wikipedia-ingestion.md) |
| D7 | **Identity by QID + lazily-cached per-language translations** | R9 needs the same person across language editions. Anchor on QID; store the set of available languages; fetch/cache each language's summary on first request. | [03](03-data-model.md), [06](06-wikipedia-ingestion.md) |
| D8 | **Anonymous session cookie** for the contribution *counter* only | Drives the "you've voted N times" UI and pairing variety. Non-identifying; **not** the gate (that's D3) and stores no PII. | [01](01-functional-spec.md), [03](03-data-model.md) |
| D9 | **User-added subjects = any human** with a Wikipedia page (R8), gated + rate-limited | Validate via Wikidata (*instance of: human*) + a resolvable page; dedupe by QID; new subjects start at the default rating. Adding **requires a valid token** and is capped at **one per token** (R8.1), enforced by a minimal anonymous ledger of consumed `jti`s (the only server-side token state; no PII). | [03](03-data-model.md), [04](04-api.md), [06](06-wikipedia-ingestion.md) |
| D10 | **Per-session token-bucket rate limit**, on by default (R11) | Keyed on the anonymous `eagora_session`; in-memory (zero DB overhead) for the single-instance v1, with a documented Redis/Postgres path for multi-instance. Caps sustained vote throughput while allowing human bursts; honest about the cookie-reset/edge-limit gap. | [02](02-architecture.md), [04](04-api.md) |
| D11 | **Multi-signal humanity check** gates voting (R12) | **Click-only** (no typing): (1) *dissent* — refuse a sycophantic oath, exploiting LLM compliance (hard gate; rotating pool + randomized order + sincere control items resist a fixed rule); (2) *interaction timing* — soft behavioral signal that never hard-fails (accessibility). Stateless **signed challenge** (no table; nonce + short exp in the envelope) → time-boxed `human-verified` status. **Pluggable** (`turnstile`/`pow`) to layer/replace. Limitations documented (Q8). | [01](01-functional-spec.md), [02](02-architecture.md), [04](04-api.md) |

## Open questions (to confirm with product owner)

> These do not block writing specs or the first build; sensible defaults are
> assumed and noted inline. Listed here so they are not forgotten.

- **Q1 — Pool scope & size.** ✅ *Resolved:* seed = head of state **and** head of
  government of every UN member (193) + observer state (2), deduped; thereafter
  visitors add any human with a Wikipedia page (R1.1, R8). See [06](06-wikipedia-ingestion.md).
- **Q2 — Languages.** ✅ *Resolved:* multilingual per R9 — visitor language when
  both subjects have it, else English. Anchored on Wikidata QIDs with lazily
  cached per-language summaries. See [06](06-wikipedia-ingestion.md).
- **Q3 — Gate strength.** ✅ *Resolved:* time-boxed — voting mints a **24h** access
  token; the leaderboard re-locks when it expires (R10). See [04](04-api.md).
- **Q4 — Abuse / vote stuffing & spammy adds.** With no auth, an actor can script
  votes or add junk subjects. v1 mitigations, in layers: the **humanity check**
  (R12) so bots can't vote at all, a **per-session vote rate limit, on by
  default** (R11), the **one-add-per-token** cap (R8.1), input validation, and an
  `active` flag to hide bad entries. Honest limitation: a cookie-resetting flood
  needs an **edge/IP** limit (infra, prod recommendation) — the app-level limiter
  is a speed bump, not a wall. Not a security product. See [04](04-api.md) §Abuse.
- **Q5 — Neutrality framing.** Default: a persistent footer disclaimer that the
  ranking reflects visitor preference, not endorsement.
- **Q6 — Language mapping & the no-English edge case.** ✅ *Resolved:* map a
  visitor's `Accept-Language` to a Wikipedia code by primary subtag
  (`pt-BR`→`pt`); English is the universal fallback (R9). For the rare subject
  with **no** English page, the fallback chain is *visitor lang → English → the
  subject's canonical language*.
- **Q7 — Abuse of user-adds.** ✅ *Resolved:* **adding is rate-limited to one per
  access token (≈ one per 24h)** — see R8.1. Combined with the human + page +
  QID-dedup checks and an `active=false` hide switch, this is v1's moderation
  posture; no human review queue.
- **Q8 — Humanity-check strength & UX (R12).** The dissent-based check stops
  naive scripts and compliant LLM bots, but a *fixed* pass-rule is learnable and
  a reasoning LLM can evaluate each prompt; it can also wrongly fail a sincere
  human who affirms the oath. The soft **interaction-timing** signal is forgeable
  by a bot that emulates human delays — a layer, not a wall. v1 mitigations:
  rotating prompt pool, randomized option order, sincere control items, short
  verification window, and the layered defenses above; timing **never hard-fails
  alone** (accessibility). *Open:* is this acceptable for launch, or should a
  managed/PoW check (the pluggable fallback) be enabled from day one — and how
  long should `human-verified` last (default: 24h, aligned with the access
  window)?

## Audience for these docs

A developer (or an agent) implementing the app, and a product owner reviewing
scope. Each later doc is self-contained but assumes the requirements above.
