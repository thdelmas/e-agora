# 11 — Belonging & Proposals

A follow-up to [10-recognition-and-pools.md](10-recognition-and-pools.md). Pools
shipped, and two problems surfaced in use:

1. **Some pools are almost empty.** The seed is ~2 figures per country (head of
   state + head of government), so a *country* pool is usually exactly two people
   — one matchup, repeated. 116 of 200 countries have exactly two living
   subjects; 67 have one.
2. **People appear where they don't seem to belong.** Pool membership is inferred
   from Wikidata geography (P27 country → P30 continent). That's geographically
   correct but *geopolitically* surprising: the transcontinental states (Türkiye,
   Kazakhstan, Azerbaijan, Georgia, Russia) land in **both** Europe and Asia, so
   the Europe pool shows figures a European wouldn't call European — while Cyprus
   and Armenia map to Asia only.

Both are the same root flaw: **membership is inferred from brittle metadata.** The
fix is to *measure* it. This is the "I don't know them" / measure-recognition-
directly lever deferred in [10 §5](10-recognition-and-pools.md) — the loop-closing
signal that says whether the whole recognition redesign worked.

## 1. Two axes: belonging and rating

Today one number (the Glicko-2 rating) carries everything. Split the concern:

| Axis | Question | Mechanism | Decides |
|------|----------|-----------|---------|
| **Belonging** | Does this figure belong to / represent this pool? | crowd **recall** (§3) | *who is in* the pool |
| **Rating** | Between two figures, who's preferred? | Glicko-2 votes (unchanged) | *how they rank* within it |

They're orthogonal. Belonging is membership; rating is ranking. A figure with a
sky-high global rating (rating axis) can have ~zero belonging to "France" (it's
not where people place them). The board keeps **one global Glicko rating**
([10 §4](10-recognition-and-pools.md)); belonging is a *new, separate* per-pool
signal that corrects the geographic seed.

## 2. The flow

```
Current:   land → choose pool → vote
Proposed:  land → choose pool → recall "who comes to mind here?" → vote
```

On entering a pool the visitor is asked, recall-framed (**not** "favorite" —
preference is what the votes already measure):

> **Who comes to mind for _{pool}_?**

A type-ahead over the pool's figures with a free-text fallback. What they name is
a **proposal**: one datum on the belonging axis. Then matchups begin —
**seeded from their pick** (their recall vs a recognition-weighted challenger), so
the recall *is* the first move of voting, not a gate in front of it.

Friction discipline (a step before voting is a funnel cost):

- Shown **once per pool entry**, not per matchup.
- **"I don't know anyone here"** skip — itself a signal (low familiarity with this
  pool) and a cue to bump the visitor up a level (country → continent → world).

## 3. Proposals may name people not yet in the pool

A recall that doesn't match an existing figure flows into the **existing add
path** (Wikipedia/Wikidata resolve → human check → create subject, `source='user'`),
scoped to the pool. So recall is also **demand-driven, pool-scoped ingestion**: a
French visitor recalling a politician we never seeded *grows the France pool*.

This is what fixes problem 1 — empty pools deepen exactly where there's attention,
instead of us trying to seed every country to depth up front. One step, both
problems: belonging **corrects** membership and proposals **deepen** it.

*Open:* an unregistered proposal is an add, today gated to one-per-access-token
([R8.1](07-roadmap.md)). Decide whether a recall may mint/spend that allowance or
queues the figure for a lighter-weight review (§7).

## 4. The belonging score

Per `(pool, subject)`. Let `n` = proposals of the subject in the pool and `N` =
total proposals into that pool. The score is a **smoothed share**, not the raw
rate, so a lone `1/1` isn't 100%:

```
belong(s, P) = (n(s,P) + a) / (N(P) + a/π₀)        # additive (Laplace) smoothing
```

…or a Wilson lower bound on `n/N` — same intent: confidence grows with evidence.
`π₀` is a neutral prior share; `a` the prior strength.

- **Bootstrap from geography.** Geographic members start with a prior so a fresh
  pool isn't empty cold; proposals are *evidence* that moves the score off that
  prior. Geography seeds, the crowd refines.
- **Threshold τ (or min count k).** Above it, a figure is a confident member.
  Below it, after enough pool traffic, a geographic member with near-zero recall
  (the Türkiye-in-Europe case) is **demoted** in that pool's draw — present but
  rare — and only hard-hidden under a strict floor. It always remains in its
  correct pools and in the global pool, so nothing is deleted and the comparison
  graph stays connected.

Belonging enters the draw as a **per-pool multiplier on the existing `R(s│v)`
recognition weight** ([10 §2](10-recognition-and-pools.md)) — it reweights *within*
a pool, it doesn't start a second rating.

## 5. Pool identity: keys, not (yet) a tree

Belonging needs a stable pool *identity*, but pools are computed predicates, not
rows. Give each scope a **canonical key**:

```
world  ·  continent:Europe  ·  country:France
```

The belonging tables key on that string. No `pools` table is required yet, and the
key already encodes a path, so it bridges cleanly to the **hierarchical navigator**
the proposal raised (World → continent → country → …, parent-left / children-right,
recursive). That navigator is a good UX evolution but a heavier, separate change (a
real parent/child pool model), and for *geography* the tree is only ~3 levels deep
with data we have (subnational is sparse — see [10 §4](10-recognition-and-pools.md)).
**Sequence belonging first**; the tree earns its keep once branches go richer than
geography (EU, office, topic), where belonging-defined membership is what fills
them. Tracked separately.

## 6. Data model

Append-only event log + maintained aggregate, mirroring votes/ratings:

- **`proposals`** — `(id, session_id, pool_key, subject_id, created_at)`. One per
  pool entry (rate-limited like votes); the raw belonging signal.
- **`pool_belonging`** — maintained counters `(pool_key, subject_id, proposals,
  updated_at)` plus per-pool totals `(pool_key, entries)` for the denominator.
  Derivable from `proposals`, materialized for the draw's sake.
- **`membership_votes`** — `(session_id, pool_key, subject_id, verdict ±1)`, one
  standing confirm/infirm verdict per session (changeable, retractable); the
  source of truth for §7.
- **`pool_membership`** — maintained `(pool_key, subject_id, confirms, infirms)`,
  recomputed from `membership_votes`; the draw and leaderboard read it.
- Subjects/sessions reused; new subjects from a recall use the existing add path.

`belong(s,P)` and the threshold are computed from these; the matchup draw and the
per-pool leaderboard read the score as a weight/filter.

## 7. Membership confidence — confirm / infirm

Recall (§2) is **positive-only**: it surfaces a *missing* member but can't argue a
*wrong* one out. Geographic membership is seeded from Wikidata P27→P30 (trusted,
imperfect — a stale colonial citizenship Wikidata never marked ended keeps Ismaïl
Omar Guelleh in the France pool). So beside every shown membership we give the
visitor a **signed verdict**: *confirm* ("belongs here") or *infirm* ("doesn't"),
plus the **reason** the figure is here ("Citizen of France · Wikidata P27"). This
is kept **separate** from the recall-share score — recall measures *who comes to
mind*; this measures *whether a geographic match is real*.

Verdicts feed a **Beta posterior mean** of "belongs", per `(pool, subject)`:

```
conf(s,P) = (confirms + α) / (confirms + infirms + α + β),   α=4, β=1
```

The prior is deliberately **trusting** — Wikidata placed them here, so with no
votes `conf = α/(α+β) = 0.8`, not 0.5. Two effects, neither of which touches the
global Glicko rating (belonging ≠ rating):

- **Soft gate (draw):** the matchup weight is multiplied by `least(1, conf/0.8)` —
  never a boost (recall does that), only a brake as the crowd argues a figure out.
- **Hard drop (membership):** below `conf < 0.25` the figure leaves the pool
  entirely (leaderboard + draw). Conservative on purpose: overriding Wikidata
  takes sustained consensus, not one or two dissents. The `world` pool is never
  voted on — everyone belongs to the world.

Surfaced on both the matchup cards and the subject detail view (per-pool list).
Rate-limited but not humanity-gated, like recall (§9).

## 8. Open decisions

- **Add gating for unregistered recalls** — spend the one-add allowance, or a
  lighter review queue? (Abuse vs growth trade-off.)
- **Score denominator** — pool *entries* vs *proposals*; smoothing constants.
- **Tree roll-up** — does belonging to `country:France` propagate to
  `continent:Europe`? Deferred with the hierarchy.
- **Membership prior/threshold tuning** — α/β and the 0.25 hard-drop floor are
  first-cut; revisit once real confirm/infirm volume arrives.

## 9. Privacy & abuse

Unchanged stance ([05](05-ranking.md), [09](09-identity-and-voting.md),
[10 §6](10-recognition-and-pools.md)). A proposal is a fact aggregated per *pool*,
never a per-visitor profile; the figure named is public; no IP, no profiling.
A proposal is **rate-limited (R11) but, unlike a vote, not humanity-gated (R12)** —
recall is the visitor's first interaction (pool entry, before any vote), so a
humanity wall there would lose the casual visitor this whole redesign exists to
serve. The abuse surface is low: the smoothed score moves `n` and `N` together, so
a single session is near-neutral and shifting belonging needs *many distinct
sessions* — which the humanity check on **votes** still discourages — while the
belonging **threshold** stops a lone actor injecting a figure into a pool. (The
*authoritative* signal, the rating, stays gated; the *soft* signal, belonging, goes
frictionless.) Unregistered-recall adds keep the human + Wikipedia-page + dedupe
checks of the existing add path.

## 9. Roadmap

Tracked as **M14–M16** in [07-roadmap.md](07-roadmap.md); each ships on its own.

- **M14 — Proposal substrate.** `proposals` + `pool_belonging`, canonical pool
  keys, `POST /api/proposals`, the recall→add path; belonging score computed. No
  draw change yet (additive).
- **M15 — Belonging-weighted membership.** The score reweights the per-pool draw
  and per-pool leaderboard; below-floor geographic members demoted; bootstrap
  prior from geography.
- **M16 — Recall flow UI.** The "who comes to mind?" step on pool entry, seeded
  first matchup, "I don't know anyone here" skip.
- **Later — Hierarchical pool navigator** (the drill-down tree), once pools are an
  explicit parent/child model rather than predicates.
