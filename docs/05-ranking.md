# 05 — Ranking & Matchup

How preferences become an order. Two parts: the **rating update** (Glicko-2)
applied on each vote, and the **pairing** that decides which A vs B to show.

## Rating model: Glicko-2

Glicko-2 is Elo's successor. Where Elo carries one number per subject, Glicko-2
carries three, and that extra state is exactly what a public, visitor-grown pool
needs:

| Symbol | Name | What it buys us |
|--------|------|-----------------|
| `R`  | rating | strength, same ~1500 scale as Elo |
| `RD` | rating deviation | **how sure** we are of `R` — shrinks with evidence, so a freshly-added subject moves fast and a settled one barely twitches |
| `σ`  | volatility | how *erratic* a subject's results have been; lets a genuinely shifting reputation move faster than a steady one |

Our only signal is still pairwise "A beat B", which is exactly Glicko-2's input.
We treat **each vote as a one-game rating period** for both subjects — the
winner and loser are each updated against the other's pre-vote state. (Glicko-2
is classically run over batched rating periods; per-vote is the incremental
specialization. We deliberately skip the between-period RD *aging* step: a
politician's appeal is roughly stationary, so we don't inflate uncertainty just
because time passed — only the volatility term inflates RD.)

### Parameters

| Parameter | Value | Notes |
|-----------|-------|-------|
| Initial rating `R₀` | **1500** | every new subject starts here |
| Initial deviation `RD₀` | **350** | maximal uncertainty for a new subject |
| Initial volatility `σ₀` | **0.06** | Glickman's default |
| System constant `τ` | **0.5** | constrains volatility change; 0.3–1.2 typical, lower = steadier |
| Scale | **173.7178** | converts display `R`/`RD` ↔ Glicko-2 internal `μ`/`φ` |

### Update formula

Convert to the internal scale `μ = (R − 1500)/173.7178`, `φ = RD/173.7178`, then
for the single opponent `j` with score `s` (1 for a win, 0 for a loss):

```
g(φ)      = 1 / √(1 + 3φ²/π²)
E         = 1 / (1 + exp(−g(φ_j)·(μ − μ_j)))
v         = 1 / ( g(φ_j)² · E · (1−E) )          (estimated variance)
Δ         = v · g(φ_j) · (s − E)                 (rating-direction quantity)
σ'        = solve Glickman's volatility equation (Illinois algorithm)
φ*        = √(φ² + σ'²)                           (RD inflated by volatility)
φ'        = 1 / √(1/φ*² + 1/v)                    (new deviation, tighter)
μ'        = μ + φ'² · g(φ_j) · (s − E)            (new rating)
```

then convert back: `R' = 1500 + 173.7178·μ'`, `RD' = 173.7178·φ'`.

Properties (sanity checks for tests):
- **Not zero-sum** (the key difference from Elo): the winner's gain need not
  equal the loser's loss — each moves in proportion to its *own* RD. An unproven
  subject swings hard; a settled one barely moves on the same result.
- RD shrinks after a played game (more evidence) and never goes non-positive.
- A win raises the winner and lowers the loser; an upset swings more than an
  expected result, for equally-certain subjects.

### Worked example (validation anchor)

The unit tests reproduce Glickman's own published example: a player at
`R=1500, RD=200, σ=0.06` (with `τ=0.5`) who in one rating period beats
`1400/30`, then loses to `1550/100` and `1700/300`, must end at:

```
R' ≈ 1464.06   RD' ≈ 151.52   σ' ≈ 0.05999
```

Matching these to the penny pins the whole pipeline (`g`, `E`, variance, the
volatility solver, and the scale conversions) to an external reference.

### Reference implementation (Go, pure function)

See [`internal/ranking/glicko.go`](../backend/internal/ranking/glicko.go). The
exported surface is value-typed and I/O-free:

```go
// Rating is a subject's full Glicko-2 state.
type Rating struct{ R, RD, Vol float64 }

// Update returns the winner's and loser's new states after W beats L,
// each updated against the other's pre-vote state (one game, 1 vs 0).
func Update(winner, loser Rating) (newWinner, newLoser Rating)
```

Internally a multi-game `apply` runs the algorithm step for step (so the
worked example above can be tested directly); `Update` calls it with one game.

### Applying an update (transactional)

On `POST /api/votes` (see [04](04-api.md)), inside one PostgreSQL transaction:
1. `SELECT … FOR UPDATE` the winner and loser rows (locked in ascending `id`
   order to avoid deadlocks), reading `rating, rd, volatility` and recording
   them as `*_rating_before` / `*_rd_before` / `*_vol_before`;
2. compute new states via `ranking.Update`;
3. `UPDATE subjects` for both: set `rating`, `rd`, `volatility`,
   `wins`/`losses += 1`, `comparisons += 1`, `updated_at = now()`;
4. `INSERT` the `votes` row (with the pre-vote snapshots);
5. `UPDATE sessions SET contributions = contributions + 1`.

The row locks serialize concurrent votes touching the same subjects, so ratings
stay consistent; votes on disjoint pairs proceed in parallel.

## Matchup pairing

Which two subjects to show. Goal: keep it interesting **and** give every subject
enough comparisons for a meaningful rating.

### Strategy: anchor + challenger (current)

Each matchup pairs one **anchor** (weighted toward fame) with one **challenger**
(weighted toward few comparisons). This came out of real feedback: drawing
*both* picks by coverage bias (the prior strategy, below) surfaced too many
pairs of mutual unknowns — "voting between two people we don't know about." A
pairwise vote only carries signal when the visitor can actually judge it, so we
guarantee one half of every pair leans recognizable.

**Anchor — the familiar half.** Weight each subject by
`wᵢ = cardinality(available_langs)`: the number of Wikipedia *language editions*
it has. That count is a strong, stable, **public and non-personal** popularity
proxy — a globally famous leader has articles in 100+ languages, an obscure one
in a handful — and it's already stored at ingest from Wikidata sitelinks, so this
needs no new data, no extra API, no refresh job, and **no user profiling**
(privacy intact: we never model who *this* visitor knows, only who the *world*
writes about). Efraimidis–Spirakis top-1 gives `P(pick = i) ∝ wᵢ`, so the slot
favors well-known figures while still rotating across the upper tier. There's no
fixed "is famous" threshold to tune — it self-adjusts to whatever the pool holds,
and degrades to near-uniform if the whole pool is obscure.

**Challenger — the coverage half.** The original bias toward subjects with **few
comparisons**, drawn over everyone but the anchor. This is the **supply side** of
the conservative leaderboard: ranking by `rating − 2·RD` buries a subject until
its `RD` shrinks, and `RD` only shrinks when the subject is shown. Coverage bias
is what makes sure unproven subjects get the votes that let them climb; without
it, conservative ordering would ossify the board to the seed pool. Pairing the
challenger against a low-`RD` anchor is a bonus — a Glicko-2 game against a
*certain* opponent tightens `RD` more per vote than two newcomers meeting would.

Both halves use **Efraimidis–Spirakis** weighted sampling without replacement —
assign each row a key `random()^(1/wᵢ)` and take the largest:

```sql
WITH anchor AS (
  SELECT id FROM subjects WHERE active
  ORDER BY power(random(), 1.0 / greatest(cardinality(available_langs), 1)) DESC LIMIT 1
), challenger AS (
  SELECT id FROM subjects WHERE active AND id NOT IN (SELECT id FROM anchor)
  ORDER BY power(random(), comparisons + 1) DESC LIMIT 1
)
SELECT id FROM anchor UNION ALL SELECT id FROM challenger
-- (wrapped in ORDER BY random() so the anchor isn't always card A)
```
For the challenger, an unseen subject (`comparisons = 0`) draws a plain uniform
key and a heavily-compared one a vanishing key. `greatest(card, 1)` guards the
all-empty-`available_langs` edge.

(Two full scans + sorts. Fine for a small pool; as visitor-added subjects grow
it, avoid the full sort — cache the active id set in memory and pick in Go, or
use `TABLESAMPLE`.)

### History: coverage-biased pair (both picks)

The first strategy weighted *both* picks by `wᵢ = 1 / (comparisons + 1)` and drew
two distinct subjects in one sort (`ORDER BY power(random(), comparisons + 1)
DESC LIMIT 2`). It rated the whole pool quickly but had a **known assortative
trade-off**: newcomers mostly met other newcomers and rarely the established
field, calibrating slowly — and, as the feedback showed, it paired unknown with
unknown too often. The anchor + challenger split above is the prescribed fix.

### Possible later: informative pairing

Pair subjects with **similar ratings** (close `E ≈ 0.5`) to extract the most
information per vote (à la Swiss tournaments). Trade-off: less variety, can feel
repetitive. Defer until volume justifies it.

### Pairing rules

- Always two **distinct** subjects (R-V1).
- Only `active = 1` subjects.
- The **living only** (`died_at IS NULL`) unless the request opts the deceased in
  (`?includeDeceased`) — see §Filtering the deceased.
- "Skip" reuses the same endpoint → a fresh independent pair (no penalty).
- No attempt in v1 to avoid showing a visitor the same pair twice (cheap to add
  later via per-session recent history if desired).

## Leaderboard ordering

```
ORDER BY (rating - 2 * rd) DESC, rd ASC, canonical_name ASC
```
- primary: the **conservative rating** `R − 2·RD` (R6 — preference-driven). This
  is Glickman's recommended display ordering: a subject climbs only once its
  rating is both high *and* well-established, so a lucky 3-vote run can't top the
  board. With `RD` floored near 50, the `−2·RD` term costs a settled subject
  ~100 points and an unproven one ~700;
- tie-break 1: lower `RD` ranks higher (more evidence);
- tie-break 2: `canonical_name` ascending (stable, deterministic for tests).

The board displays the raw `R` and surfaces `RD` so the UI can mark a
high-deviation entry **provisional** (see [01](01-functional-spec.md)).

### Cold-start note

A new subject starts at `1500 ± 350`, so its conservative rating (`~800`) puts
it near the *bottom* until votes tighten its `RD` — by design, unproven subjects
don't masquerade as ranked. Because every subject starts at the same `RD`, the
day-one board order still tracks raw rating, then differentiates as evidence
accrues.

## Filtering the deceased

The pool mixes the living and historical figures, but a ranking of "who would you
rather have as a leader" reads most naturally as a contest among the living. So
both the matchup draw and the leaderboard **default to the living** and treat the
deceased as opt-in.

- **Signal.** Wikidata `P570` (date of death), read at ingest next to `P31`
  (docs/06-wikipedia-ingestion.md §Step 2) and stored as `subjects.died_at DATE`.
  A subject is *deceased* iff `died_at IS NOT NULL`. The rare `P570` "somevalue"
  snak (known dead, date unknown) carries no date and is left unflagged.
- **Filter.** Both `RandomPair` and `TopByRating` add `AND (includeDeceased OR
  died_at IS NULL)`. Default (`includeDeceased = false`) → living only; the
  viewer's toggle flips it for **both** surfaces at once, so the pool they vote on
  matches the ranking they read.
- **Ratings are untouched.** The filter is purely a read-time visibility gate:
  `died_at` never affects the Glicko-2 math, and a deceased subject keeps every
  rating and vote it earned. Voting on a deceased figure (toggle on) records
  normally — `RecordVote` gates on `active`, not `died_at` — so historical figures
  accumulate a real conservative rating to compare against the living.
- **Backfill.** `died_at` is nullable; existing rows fill in on the next
  `EAGORA_SEED=force`, which re-ingests every subject (ratings/votes preserved by
  the upsert). Until then, un-backfilled rows read as living.

## Enhancements backlog (not v1)

- **Rating periods**: batch votes into periodic Glicko-2 updates (instead of the
  per-vote incremental form) and add the between-period RD-aging step — more
  faithful to the paper if vote volume ever makes per-vote updates noisy.
- **Volatility-aware display**: surface `σ` (e.g. a "trending" badge) when a
  subject's reputation is genuinely shifting.
- **Recompute endpoint/CLI**: replay `votes` (which snapshot full pre-vote
  state) to rebuild ratings after a parameter change — enabled by the
  append-only `votes` log, see [03](03-data-model.md).
