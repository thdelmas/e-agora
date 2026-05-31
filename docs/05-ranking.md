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

### Strategy: coverage-biased pair (current)

Bias selection toward subjects with **few comparisons** so the whole pool gets
rated — and its `RD` tightened — quickly. This is the **supply side** of the
conservative leaderboard: ranking by `rating − 2·RD` buries a subject until its
`RD` shrinks, and `RD` only shrinks when the subject is shown. Coverage bias is
what makes sure unproven subjects actually get the votes that let them climb;
without it, conservative ordering would ossify the board to the seed pool.

Weight each subject by `wᵢ = 1 / (comparisons + 1)` and draw two distinct ones.
We realize this with **Efraimidis–Spirakis** weighted sampling without
replacement — assign each row a key `random()^(1/wᵢ)` and take the two largest:

```sql
SELECT id FROM subjects WHERE active
ORDER BY power(random(), comparisons + 1) DESC LIMIT 2;
```
An unseen subject (`comparisons = 0`) draws a plain uniform; a heavily-compared
one draws a vanishing key and is rarely shown. With an all-zero pool this is
exactly uniform random, so it degrades gracefully and needs no special-casing
for a fresh deploy.

(Full scan + sort, like the prior `ORDER BY random()`. Fine for a small pool;
as visitor-added subjects grow it, avoid the full sort — cache the active id set
in memory and pick two in Go, or use `TABLESAMPLE`.)

> **Known trade-off — assortative pairing.** Because *both* picks are weighted,
> newcomers mostly meet other newcomers and rarely the established field, so
> their ratings calibrate against the veterans slowly. If that matters, weight
> only one pick by coverage and draw the opponent uniformly (often an
> established, low-`RD` subject) — a Glicko-2 game against a *certain* opponent
> tightens `RD` more per vote anyway. Cheap to switch; left as the literal
> spec for now.

### Possible later: informative pairing

Pair subjects with **similar ratings** (close `E ≈ 0.5`) to extract the most
information per vote (à la Swiss tournaments). Trade-off: less variety, can feel
repetitive. Defer until volume justifies it.

### Pairing rules

- Always two **distinct** subjects (R-V1).
- Only `active = 1` subjects.
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

## Enhancements backlog (not v1)

- **Rating periods**: batch votes into periodic Glicko-2 updates (instead of the
  per-vote incremental form) and add the between-period RD-aging step — more
  faithful to the paper if vote volume ever makes per-vote updates noisy.
- **Volatility-aware display**: surface `σ` (e.g. a "trending" badge) when a
  subject's reputation is genuinely shifting.
- **Recompute endpoint/CLI**: replay `votes` (which snapshot full pre-vote
  state) to rebuild ratings after a parameter change — enabled by the
  append-only `votes` log, see [03](03-data-model.md).
