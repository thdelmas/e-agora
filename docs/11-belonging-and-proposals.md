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
- Subjects/sessions reused; new subjects from a recall use the existing add path.

`belong(s,P)` and the threshold are computed from these; the matchup draw and the
per-pool leaderboard read the score as a weight/filter.

## 7. Open decisions

- **Below-floor handling** — demote (downweight) vs hard-hide a low-belonging
  geographic member. Lean: demote, hard-hide only under a strict floor.
- **Add gating for unregistered recalls** — spend the one-add allowance, or a
  lighter review queue? (Abuse vs growth trade-off.)
- **Score denominator** — pool *entries* vs *proposals*; smoothing constants.
- **Tree roll-up** — does belonging to `country:France` propagate to
  `continent:Europe`? Deferred with the hierarchy.

## 8. Privacy & abuse

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
