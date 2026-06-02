# 10 — Recognition & Pools

The single most common piece of feedback: **"I'm asked to vote between two people I've
never heard of."** This document diagnoses why, and lays out the design and phased
roadmap that fixes it.

## 1. The diagnosis: a wrong-axis problem, not a tuning problem

The pool is *every UN leader on Earth* (~380 seeded, growing daily via the Wikidata
sync). Any one human recognizes maybe 20–50 of them — their own country's figures plus
a dozen global names. So for any given visitor, the **vast majority of the pool is
foreign and unknown**. No weighting *within* a single global pool fully fixes this,
because the problem is the **axis**: the board measures *global* notability, while
recognition is *local* (linguistic / regional / "famous enough that everyone knows
them").

Today's [`RandomPair`](../backend/internal/store/queries.go) tries to dodge this with an
**anchor + challenger** scheme: the anchor is weighted by `cardinality(available_langs)`
(sitelink count = *global* fame). Three reasons it still serves two unknowns:

1. **Wrong axis.** Sitelink count is worldwide notability. A French visitor doesn't know
   a globally-notable Indonesian leader. The proxy should be *local* recognition.
2. **Too gentle.** Efraimidis–Spirakis with a *linear* weight makes a famous figure only
   ~20–40× likelier *per subject* than an obscure one — but there are far more obscure
   subjects, so the long tail still wins the anchor slot a large fraction of the time.
3. **The challenger is deliberately obscure** (coverage bias → low-comparison →
   unproven). So even when the anchor *is* famous, you get "famous vs. nobody."

A subtle trap rules out the cheap fix: **"has an article in the visitor's language" is
too weak a filter for the major languages.** `fr`/`en`/`es`/`de` Wikipedia have an
article for essentially *every* head of state on the planet, including ones nobody in
those language spheres could name. Having an article ≠ being recognized. What
discriminates "Macron (millions of French views)" from "the Kyrgyz president (a few
hundred French views)" is **attention**, i.e. **Wikipedia pageviews per language**.

## 2. The relevance model: one signal, three levers

Everything keys off **per-language Wikipedia pageviews** — actual attention, and
privacy-preserving (it is a fact about the *article*, never the visitor). All three
levers derive from that one signal, so we ingest a single new thing:

| Lever | Definition | Catches | In one word |
|-------|-----------|---------|-------------|
| **Local attention** | `local_views(s,v)` — views of *s* in language *v* | Macron is huge in `fr`; the Kyrgyz president is a rounding error in `fr` | language |
| **Global fame** | `global_views(s)` — total views across all languages | Trump / Putin / the Pope, recognizable *everywhere* regardless of *v* | very-famous |
| **Sphere affinity** | `local_views(s,v) / global_views(s)` — *v*'s *share* of *s*'s attention | a francophone-Africa president whose (modest) audience is concentrated in `fr` belongs to the French sphere | region |

The sphere lever is why we need no separate region table for the recognition weight:
**attention-share *is* a region proxy.** (Explicit region *pools* — §4 — still want a
structured country, re-added for that purpose.)

**Recognition score**, visitor-relative, in log space (pageviews are Zipfian):

```
R(s│v) = α·log(1+local_views) + β·log(1+global_views) + γ·share·log(1+local_views)
```

`α / β / γ` are config knobs. The `log(1+local_views)` factor on the share term is a
volume floor, so a genuinely obscure person who happens to be read 100% in `fr` is not
over-promoted. Because the **β term is visitor-independent**, the recognizable set is
never empty even for a thin language — the blend is its own graceful fallback. When
pageview data is absent entirely (fresh DB, before the first sync), the score degrades
to the existing `cardinality(available_langs)` weight so the draw still works.

## 3. The draw

The matchup handler already resolves the visitor's language (`lang.Pick` →
`Accept-Language` / `?lang=`) — but only *after* the pair is drawn, for display. We push
that language *into* the draw:

- **Draw both subjects weighted by `R(s│v)`** (reusing the log-space Efraimidis–Spirakis
  trick). Both ∝ R ⇒ both are people this visitor plausibly recognizes, via *any* lever.
  This is what kills "two unknowns."
- **Discovery slot (~15–20%):** keep the coverage-biased challenger (`1/(comparisons+1)`)
  for a minority of draws, so the long tail still accrues comparisons (its Glicko RD
  shrinks and it eventually gets ranked) — always against a *recognizable* anchor, never
  two nobodies. The challenger always honors the active pool (§4), so an explicit
  selection never leaks an out-of-pool figure.

## 4. Pools: agency instead of inference

Instead of only *inferring* what a visitor knows, let them **pick a context** where they
recognize the players. The levers above become the **axes they choose along**. (The
existing living/deceased toggle is already a visitor-selected pool dimension; this
generalizes it.)

| Axis | Choices | Source |
|------|---------|--------|
| **Region / country** | World · Europe · Africa · a country | Wikidata P27/P17 (re-added) + continent |
| **Fame tier** | "premier league" (globally famous) · all | `global_views` |
| **Office tier** | heads of state · heads of government · ministers | Wikidata P39 → small enum |
| **Status** | in office · historical | P39 end-date / `died_at` (have it) |

A figure's `continent` is a **set**, not a single label: a country can straddle a
continental boundary (Russia → Europe + Asia, Egypt → Africa + Asia, Türkiye, …),
and such a figure belongs in *every* continent pool it spans. Only the contiguous
transcontinental states get multiple continents — a country's Wikidata P30 also
names its *overseas territories'* continents (France spans Europe + Africa +
Oceania + Antarctica), so for everyone else we keep just the primary continent.
The region filter is therefore a containment test (`continent @> ARRAY[region]`).
Country of citizenship (P27) resolves to the *current* one — a former, ended
citizenship (e.g. the Soviet Union for a post-1991 Russian leader, marked by a
P582 end-time qualifier and outranked by the preferred claim) is skipped.

**Ranking stays one global Glicko rating.** Per-pool leaderboards are *filtered views*
over that one rating column (`WHERE continent @> ARRAY[region] AND global_views≥…`) — ranking *within* a
pool is a query filter, not a separate rating system. That validity rests on two things:
(1) we **store the axis fields** to filter on, and (2) the global comparison graph stays
**connected** — Glicko ratings are only comparable within a connected component, and
siloed voting would let the per-pool slices drift onto incomparable scales. Explicit
pools are kept *strict* (a region selection only ever shows that region) for clean UX;
connectivity instead comes from the **default pool**, whose recognition + discovery draws
inherently span every continent and so bridge the regions together.

The default pool is a smart **For You** (recognition-weighted, global), because most
visitors never touch the picker — so the implicit model of §2–§3 still has to be good.

### The home region: a soft bias, asked once

To land a visitor in the most accurate default pool without IP geolocation (§6) or
fragmenting the rating (above), we **ask instead of infer** — a one-time onboarding
step ("Where do you follow politics most closely?") shown alongside the welcome,
before the agora opens. The continent it captures is the visitor's **home region**,
and it is a **soft bias, not the strict pool filter**:

- In the draw it adds a flat `region_boost` to the recognition score `R(s│v)` of every
  subject in that continent (a new `α/β/γ`-style knob, `EAGORA_RECO_REGION`). Flat and
  additive in log-space `R`, it lifts the *modest local* figures a visitor knows
  (`R≈5 → ×1.5`) far more than the globally-famous (`R≈20 → ×1.1`) who don't need it.
- It **excludes no one.** The coverage-biased discovery challenger (§3) stays
  region-blind, so the default draw still spans every continent and the single Glicko
  scale stays comparable. This is the whole reason it's a bias and not a `WHERE` —
  a strict home pool would silo the graph; the strict filter remains the explicit,
  opt-in PoolPicker selection.

The onboarding chip is **pre-highlighted from the browser's volunteered language**
(`navigator.languages`; a region subtag like `pt-BR`/`en-AU` is the strong signal,
the bare language a weak fallback, anything global/ambiguous → "🌍 Whole world"), so
the common path is a single confirming tap. The choice lives in `localStorage` and
travels only as the per-request `?home=` flag — never a stored visitor profile (§6).

## 5. Levers considered but deferred

- **Pageview *trend*** (7d vs 90d) — "who's in the news right now," nearly free since we
  already pull pageviews. Strong follow-up for freshness.
- **Rating proximity** (Glicko) — pair similar-rated subjects for more informative,
  closer contests.
- **"I don't know them" skip** — *measures* recognition directly (aggregated per
  language, privacy-safe) instead of inferring it; closes the loop and gives a metric for
  whether this whole redesign worked.
- **Avoided:** IP geolocation (crosses the no-profiling line — language is the
  volunteered proxy) and party affiliation (injects partisanship into a neutral board).

## 6. Privacy

Unchanged stance ([05](05-ranking.md), [09](09-identity-and-voting.md)): pageviews are
article facts, not visitor facts; the visitor's language is volunteered, never
geolocated; pools are visitor-selected. No per-visitor profile is built or stored.

## 7. Roadmap

Tracked as milestones **M9–M13** in [07-roadmap.md](07-roadmap.md). Each ships on its own;
**M10 alone fixes the headline complaint.**

- **M9 — Pageview substrate.** Migration `subject_pageviews` + `subjects.global_views`;
  ingest per-language pageviews over a served-languages set during the sync; recompute
  `global_views`. No behavior change yet (additive).
- **M10 — Recognition-weighted draw.** Pass the visitor language into `RandomPair`; weight
  by `R(s│v)`; keep the discovery slot; graceful fallback when data is absent. *The fix.*
- **M11 — Pools.** Re-add `country` + add `office_tier` / in-office; filter the draw by a
  selected pool; reserve the discovery slot as the cross-pool bridge.
- **M12 — Per-pool leaderboards.** Filter params on `/api/leaderboard` as views over the
  global rating.
- **M13 — UI.** Pool picker on the voting view + filters on the leaderboard.
