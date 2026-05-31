# 01 — Functional Specification

Defines *what the app does* from the visitor's point of view: the flows, the
screens, and the rules that govern them. Implementation lives in later docs.

## Actors

- **Visitor** — anyone who opens the site. Anonymous (R3). Votes, and may **add
  new subjects** (R8). The only human actor.
- **System** — the backend that pairs subjects, records votes, mints access
  tokens, ingests added subjects, and ranks.

The pool is **seeded** with UN-country leaders ([06](06-wikipedia-ingestion.md))
and then **grown by visitors** (R8). There is no admin UI in v1; bad entries are
hidden via an `active` flag out-of-band.

## The core loop

```
                 ┌──────────────────────────────────────────────────┐
                 │                                                  │
   ┌──────────┐  │  ┌───────────┐   vote   ┌──────────────────┐     │
   │  Visitor │──┼─▶│  Matchup  │─────────▶│ Recorded + rated │─────┘
   │  lands   │  │  │  A  vs  B │          │ 24h access token │
   └──────────┘  │  └─────┬─────┘          │ (re)issued       │
                 │    ▲   │ add (needs token,      └────────┬─────────┘
                 │    │   ▼  once per token)                │ valid token?
                 │    │ ┌───────────────┐                   ▼
                 │    │ │ Add a subject │            ┌────────────┐
                 │    │ │ (Wikipedia)   │            │  may view  │
                 │    │ └───────────────┘            │ Leaderboard│
                 │    └──"vote again"────────────────└─────┬──────┘
                 │       (re-vote after expiry; fixed 24h)  │ token expires (24h)
                 └──────────────────────────────────────────┘ → re-lock
```

1. Visitor lands on `/`.
2. System presents a **matchup**: two distinct subjects A and B (name, image,
   one-line description, Wikipedia link) — rendered in the visitor's language
   when possible (R9; see §Internationalization).
3. Visitor clicks the one they prefer. If the session is not **human-verified**
   (R12), a quick **humanity check** appears first (S4): *refuse the loyalty
   oath* to prove you're a thinking human. Passing grants a time-boxed
   human-verified status (re-checked only when it lapses). The **vote** is then
   recorded — winner / loser — ratings update, and — if the visitor doesn't
   already hold a valid token — the system **issues a 24-hour access token**
   (R10). The window is **fixed**: extra votes within it don't extend it; after
   it expires, the next vote starts a fresh 24h.
4. A fresh matchup loads immediately, so the visitor can keep going.
5. While the visitor holds a **valid access token**, the **Leaderboard** is
   unlocked. When it expires, the leaderboard re-locks until the visitor votes
   again.
6. A visitor holding a valid token may also **add a subject** (any human with a
   Wikipedia page, R8) — **once per token** (R8.1) — which then enters the pool.

## Screens

### S1 — Matchup (route `/`)

The landing and default screen.

**Contents**
- Project wordmark **e-agora** and a one-line tagline.
- Prompt: **"Who do you prefer?"** (localized to the display language).
- Two **PoliticianCards** side by side (stacked on narrow screens):
  - image (from Wikipedia), name, one-line description, country (if known),
    and a small "Wikipedia ↗" link to the **display-language** page (opens in a
    new tab; satisfies R2 visibly).
- A **language note** when the English fallback was applied (R9): a subtle
  *"Shown in English — one of these isn't available in <language>."*
- A subtle **"Skip / show another pair"** action (no vote recorded).
- An **"+ Add someone"** entry point that opens the Add-a-subject flow (S3).
  Enabled only with a valid token **and** an unused add allowance; otherwise
  disabled with a tooltip (*"Vote to unlock adding"* / *"You've already added
  someone in this 24h window"*).
- A **contribution counter**: e.g. *"You've contributed 3 votes."*
- A **Leaderboard** nav entry, driven by access-token state (R10):
  - **locked** (no valid token): shown but disabled, tooltip *"Vote once to
    unlock the rankings for 24 hours."*
  - **unlocked** (valid token): active link, with the remaining window, e.g.
    *"Rankings · expires in 23h"*.
- Persistent **neutrality disclaimer** in the footer (see §Rules R-N).

**Interactions**
- Clicking a card (or its "Prefer" button) casts a vote for that subject and
  loads the next matchup. The interaction should feel instant: optimistic UI is
  allowed, but the next matchup comes from the server.
- "Skip" loads a new matchup without recording anything.
- Keyboard: **←/→** select left/right; **S** skips. (Accessibility nicety.)

**Empty / error states**
- If fewer than 2 subjects exist, show a friendly "The agora is still being set
  up — check back soon." message.
- On vote failure, keep the matchup, surface a non-blocking error toast, allow
  retry.
- On `429 rate_limited` (R-V5/R11), show *"Whoa — slow down a moment."* and
  briefly disable the vote buttons until `Retry-After` elapses, then re-enable.
  The shown matchup is preserved.

### S2 — Leaderboard (route `/leaderboard`)

**Gating (R4 + R10)** — this route is reachable only while the visitor holds a
**valid 24h access token** (minted by voting). A visitor with no token, or an
**expired** one, who navigates here (e.g. direct URL or after 24h) is
**redirected to `/`** with a message *"Vote once to unlock the rankings for 24
hours."* The gate is enforced **on the server** (the leaderboard API returns
`403 access_required` / `403 access_expired`) and mirrored **on the client**
(route guard reading the token's known expiry) for UX.

A subtle banner shows the **remaining access window** (e.g. *"Access expires in
2h — re-vote after it lapses to start a new 24h"*) so expiry is never a surprise.
(The window is fixed; voting during it doesn't extend it.)

**Contents**
- Title: **"World rankings"** + subtitle explaining it reflects visitor
  preferences.
- Ordered list, rank 1..N:
  - rank number, image, name, country, **rating**, and **wins–losses** (or
    "matchups" count).
  - subtle highlight for subjects the visitor personally voted for (optional,
    nice-to-have; requires per-session vote history — out of scope for v1 unless
    cheap).
- "Total votes cast (all visitors): N" headline stat.
- A prominent **"Keep voting"** button back to `/`.
- Same footer disclaimer.

**Pagination** — default show top 100; the pool grows as visitors add subjects
(R8), so paginate / infinite-scroll beyond that, with a result count.

### S3 — Add a subject (modal over `/`, or route `/add`)

Lets a visitor expand the pool (R8). Opened from the **"+ Add someone"** entry.
**Requires a valid access token and an unused add allowance** (R8.1): a visitor
who hasn't voted is prompted to vote first; a visitor who already added someone
in the current 24h window sees *"You can add one person per 24 hours — vote
again after your access renews."*

**Contents & interaction**
- A single input: paste a **Wikipedia URL** *or* type a **name** to search.
  - On a URL, the system resolves the page → Wikidata QID.
  - On free text, the system offers Wikipedia search suggestions (name +
    thumbnail + short description) to disambiguate before adding.
- A confirmation preview: image, name, description, the resolved Wikipedia link.
- **Eligibility checks** surfaced inline (R8):
  - must resolve to a real Wikipedia page (R2);
  - must be a **human** (Wikidata *instance of: human*);
  - must not already be in the pool (dedupe by QID).
- **Submit** → on success, a brief confirmation (*"Added — they'll start
  appearing in matchups."*) and the visitor returns to voting; the new subject
  enters the pool at the default rating.

**Error states** (mirror the API, [04](04-api.md))
- no / expired token → *"Vote once to unlock adding."*
- add already used this window → *"You can add one person per 24 hours."*
- not found / not a Wikipedia page → *"We couldn't find a Wikipedia page for
  that."*
- not a person → *"e-agora is for people — that page isn't a person."*
- already exists → *"They're already in the agora"* (optionally link to vote).

Adding consumes the token's **single** add allowance (R8.1) and does **not**
mint or refresh the token — only voting starts/holds the 24h window (R10).

### S4 — Humanity check (modal over `/`)

Appears when an un-verified session tries to vote (R12). Anonymous; proves "a
thinking human," not *who*. Bots-can't-vote is the goal.

Multi-signal, **click-only** (no typing — accessible by design).

**Signal 1 — refuse the loyalty oath (hard / semantic)**
- Framing copy, civic and a little wry: *"The agora is for citizens who think
  for themselves. Don't just agree — decide."*
- A **sycophantic political "oath"** drawn from a rotating pool, e.g. *"I fully
  trust that all world leaders serve humanity's interest before their own."*
- Choices (order randomized): the visitor **passes by refusing/dissenting**
  (e.g. *"I won't swear to that"*) rather than affirming. A compliant,
  instruction-following bot tends to affirm → fails.
- **Anti-fixed-policy**: occasionally a **sincere control statement** is shown
  where the honest action is to *agree*, so an "always refuse" bot also fails;
  the prompt pool and option order rotate per challenge.

**Signal 2 — interaction timing (soft / behavioral, no typing)**
- The client reports the natural timing of the interaction — how long between the
  oath appearing and the choice, whether the click is *suspiciously instant*, and
  coarse pointer/touch cadence. Scripted/instant patterns lower confidence and
  trigger another round; they **never hard-fail on their own** (a slow or
  assistive-tech user must not be blocked). Timing is ephemeral and **not stored**.

**Outcomes**
- **Pass** if the dissent choice is correct (Signal 1) and the interaction timing
  isn't bot-flagged (Signal 2): the session becomes **human-verified** for a
  configurable window (default 24h), the modal closes, and the pending vote
  proceeds automatically.
- **Wrong choice** → a fresh challenge with a different prompt. Repeated failures
  fall under the rate limit (R11).

**Notes**
- Re-checked only when the window lapses (Q2) — not every vote.
- **Accessibility first**: click-only, keyboard- and screen-reader-friendly; the
  satire lives in the copy, never in color/image alone; Signal 2 can only *add*
  friction, never single-handedly block a real person.
- **Honest limitation** (surfaced in [04](04-api.md) §Abuse, Overview Q8): this
  deters naive scripts and compliant LLM bots, not a determined adversary who
  studies the pass-rule or emulates human timing. Pluggable so a stronger check
  can be layered.

## Rules

- **R-G1 (gate).** Leaderboard data is inaccessible without a **valid access
  token**. Enforced server-side; client redirect is convenience only.
- **R-G2 (fixed 24h TTL).** A vote mints a token valid for **24 hours** only when
  none is currently valid (the window is **fixed, not rolling**); when it expires
  the leaderboard re-locks until the visitor votes again (R10).
- **R-V1 (distinct pair).** A matchup always contains two **different** subjects.
- **R-V2 (winner/loser).** A vote names exactly one winner and one loser, both
  belonging to the matchup that was shown.
- **R-V3 (no self-vote / no tie).** No ties; the visitor must pick one.
- **R-V4 (skips are free).** Skipping records nothing, mints no token.
- **R-V5 (rate limit).** Voting is rate-limited per session (R11). When the limit
  is hit, the vote is refused with a clear, brief "slow down" message and voting
  is disabled until the `Retry-After` moment; nothing is recorded.
- **R-V6 (humans only).** A vote is accepted only from a **human-verified**
  session (R12). An un-verified vote attempt triggers the humanity check (S4);
  the vote proceeds only after it passes. The check is **not** auth (R3).
- **R-I1 (no mixed language).** A matchup renders both subjects in one display
  language: the visitor's language if **both** have it, else English (R9).
- **R-I2 (links match display).** Each card's Wikipedia link points to the page
  in the display language being shown.
- **R-ADD1 (human + page).** An added subject must resolve to a Wikipedia page
  (R2) and be a human (Wikidata *instance of: human*) (R8).
- **R-ADD2 (dedupe).** Subjects are unique by Wikidata QID; re-adding an existing
  subject is rejected, not duplicated.
- **R-ADD3 (adding is gated).** Adding requires a **valid access token** (so the
  visitor must have voted first); adding does **not** mint or refresh a token —
  only voting does (R10).
- **R-ADD4 (one add per token).** A token permits **exactly one** add (≈ one per
  24h, R8.1); a second attempt with the same token is rejected.
- **R-N (neutrality).** Every screen carries: *"e-agora reflects the aggregated
  preferences of anonymous visitors. It is not an endorsement, poll, or
  measure of merit."* This is a product requirement, not legal advice.
- **R-A (anonymity).** No personal data is requested or stored. The session
  cookie is opaque and non-identifying; the access token carries **no**
  identifier and is not stored server-side (see [04](04-api.md)).

## Visitor journeys

### J1 — First-time visitor (happy path)
1. Lands on `/`; leaderboard nav is **locked**.
2. Sees A vs B (in their language, or English if one lacks it); clicks A.
3. Since the session isn't human-verified, the **humanity check** (S4) appears;
   the visitor *refuses the loyalty oath* → human-verified for 24h. The vote then
   records; counter → 1; a **24h access token** is issued; leaderboard nav
   **unlocks** ("expires in 24h"); next matchup shown.
4. Votes a few more times — no further humanity check this window; neither the
   verification nor the access window slides with the extra votes.
5. Opens **Leaderboard**; sees standings; clicks **Keep voting**; returns to `/`.

### J2 — Returning visitor, token still valid
1. Lands on `/`; a non-expired token is present; leaderboard **unlocked**.
2. May go straight to the leaderboard or keep voting.

### J2b — Returning visitor, token expired (>24h)
1. Lands on `/`; the token has expired; leaderboard is **re-locked**.
2. Opening `/leaderboard` redirects to `/` with *"Vote once to unlock for 24h."*
3. A single vote re-issues the token and unlocks again.

### J3 — Direct leaderboard access with no token
1. Visitor opens `/leaderboard` directly, never voted.
2. Client guard + server `403 access_required` → redirected to `/`.

### J4 — Cookies disabled
1. Without cookies, neither the session counter nor the access-token cookie can
   persist, so the gate cannot be satisfied. The app shows: *"Please enable
   cookies to contribute and view rankings."* (Acceptable v1 limitation; no auth
   fallback.)

### J5 — Adding a subject
1. Visitor (holding a valid token, add allowance unused) clicks **"+ Add
   someone"**, pastes a Wikipedia URL (or searches).
2. System validates: valid token + unused allowance, real page, is a human, not
   already present.
3. On success, the person joins the pool and starts appearing in matchups; the
   token's single add allowance is now spent (R-ADD4) until the window renews.
   (No token is minted — adding doesn't unlock anything, R-ADD3.)

### J6 — Language fallback
1. A French visitor gets a matchup where A has a French page but B does not.
2. Per R9, **both** cards render in **English**, with the note *"Shown in
   English — one of these isn't available in French."*

### J7 — Humanity check (and a bot that fails it)
1. A first-time visitor clicks to vote → the humanity check (S4) appears.
2. They refuse the oath → human-verified for 24h; the vote proceeds. They aren't
   asked again until the window lapses.
3. A scripted client that affirms the oath (or can't render the challenge) is
   **refused** and never records a vote (R12) — and, lacking a human-verified
   session, gets `403 human_check_required` from the API.

## Internationalization (R9)

The UI chrome and the subject content are localized independently:

- **Subject content** (name, description, image, Wikipedia link) is rendered in
  the **display language** chosen per matchup by R9's rule:
  - the **visitor's language** if **both** subjects have a page in it;
  - otherwise **English** for both.
  The server resolves this and returns localized content + which language it
  used (and whether a fallback happened) — see [04](04-api.md) `GET /api/matchup`.
- **Visitor language** comes from the browser `Accept-Language` (primary subtag,
  e.g. `pt-BR`→`pt`), with an optional explicit override the client can persist
  and send (`?lang=`). Mapped to a Wikipedia language code.
- **UI chrome** (buttons, prompts, the fallback note) is localized client-side.
  v1 may ship English chrome + a small set of high-coverage languages; the
  subject-content rule above is the part the requirement (R9) pins down.
- **No mixed-language matchups** (R-I1): the visitor never sees A in one language
  and B in another.

## Tone & framing

Light, civic, a little playful ("the agora"), but neutral about the subjects
themselves. Avoid language implying the ranking measures competence or virtue;
it measures *preference among visitors*.

## Accessibility & responsiveness

- Mobile-first; two cards stack vertically on narrow viewports.
- All actionable elements keyboard-reachable; images have alt text (subject
  name); color is never the only signal.
- Target: usable on a phone in portrait.
