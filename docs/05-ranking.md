# 05 — Ranking & Matchup

How preferences become an order. Two parts: the **rating update** (Elo) applied
on each vote, and the **pairing** that decides which A vs B to show.

## Rating model: Elo

Elo expresses each subject's strength as a single number and updates it after
every head-to-head, moving rating from the loser to the winner by an amount that
depends on how *surprising* the result was. It is the right tool here: our only
signal is pairwise "A beat B", which is exactly Elo's input.

### Parameters

| Parameter | Value (v1) | Notes |
|-----------|------------|-------|
| Initial rating `R₀` | **1500** | every new subject starts here |
| K-factor `K` | **32** | update step size; larger = faster, noisier |
| Scale | **400** | standard Elo logistic scale |

> `K=32` is a deliberate v1 default: responsive enough to sort a fresh pool
> quickly. If ratings look jittery once vote volume is high, lower `K` (e.g. 24
> or 16), or make `K` decay with `comparisons` (see §Enhancements).

### Update formula

For a vote where **W** beats **L**, with current ratings `R_W`, `R_L`:

```
Expected score for W:   E_W = 1 / (1 + 10^((R_L − R_W) / 400))
Expected score for L:   E_L = 1 − E_W

Actual scores:          S_W = 1,  S_L = 0

New ratings:
  R_W' = R_W + K · (S_W − E_W)
  R_L' = R_L + K · (S_L − E_L)
```

Properties (sanity checks for tests):
- Conservation: `(R_W' − R_W) = −(R_L' − R_L)` → total rating is preserved.
- Upset (low-rated beats high-rated) → large swing; expected win → small swing.
- A win never decreases the winner's rating; a loss never increases the loser's.

### Worked example

`R_W = 1500`, `R_L = 1500`, `K = 32`:
```
E_W = 1/(1+10^(0/400)) = 0.5
R_W' = 1500 + 32·(1 − 0.5) = 1516
R_L' = 1500 + 32·(0 − 0.5) = 1484
```
A perfectly even matchup moves ±16. If instead `R_W = 1300`, `R_L = 1700`
(big upset):
```
E_W = 1/(1+10^(400/400)) = 1/(1+10) ≈ 0.0909
R_W' ≈ 1300 + 32·(1 − 0.0909) ≈ 1329.1
R_L' ≈ 1700 + 32·(0 − 0.9091) ≈ 1670.9   (≈ ±29)
```

### Reference implementation (Go, pure function)

```go
// internal/ranking/elo.go
package ranking

import "math"

const (
    DefaultRating = 1500.0
    KFactor       = 32.0
    scale         = 400.0
)

// Expected score of A against B.
func expected(rA, rB float64) float64 {
    return 1.0 / (1.0 + math.Pow(10, (rB-rA)/scale))
}

// Update returns the winner's and loser's new ratings after W beats L.
func Update(rWinner, rLoser float64) (newWinner, newLoser float64) {
    eW := expected(rWinner, rLoser)
    newWinner = rWinner + KFactor*(1.0-eW)
    newLoser  = rLoser  + KFactor*(0.0-(1.0-eW))
    return
}
```

Unit tests assert: even matchup → ±16; conservation; monotonicity; upset
magnitude > expected-result magnitude.

### Applying an update (transactional)

On `POST /api/votes` (see [04](04-api.md)), inside one PostgreSQL transaction:
1. `SELECT … FOR UPDATE` the winner and loser rows (locked in ascending `id`
   order to avoid deadlocks), reading `rating` and recording them as
   `*_rating_before`;
2. compute new ratings via `ranking.Update`;
3. `UPDATE subjects` for both: set `rating`, `wins`/`losses += 1`,
   `comparisons += 1`, `updated_at = now()`;
4. `INSERT` the `votes` row;
5. `UPDATE sessions SET contributions = contributions + 1`.

The row locks serialize concurrent votes touching the same subjects, so ratings
stay consistent; votes on disjoint pairs proceed in parallel.

## Matchup pairing

Which two subjects to show. Goal: keep it interesting **and** give every subject
enough comparisons for a meaningful rating.

### v1 strategy: uniform random pair

Pick two **distinct active** subjects uniformly at random.
- Pros: dead simple; unbiased; trivially correct.
- Cons: with many subjects, coverage is slow and high-rated/low-rated extremes
  meet rarely.

```sql
SELECT id FROM subjects WHERE active ORDER BY random() LIMIT 2;
```
(Fine for a small pool. As visitor-added subjects grow the pool, avoid the full
sort — e.g. cache the active id set in memory and pick two in Go, or use
`TABLESAMPLE`.)

### v1.1 enhancement: coverage bias (recommended early)

Bias selection toward subjects with **few comparisons** so the whole pool gets
rated quickly:
1. pick subject A weighted by `1 / (comparisons + 1)` (favor under-compared);
2. pick subject B similarly, `B ≠ A`.

Uses `INDEX(active, comparisons)`. Keeps things fair without making matchups
predictable.

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

## Ranking (the leaderboard order)

```
ORDER BY rating DESC, comparisons DESC, canonical_name ASC
```
- primary: Elo `rating` (R6 — preference-driven);
- tie-break 1: more `comparisons` ranks higher (more evidence);
- tie-break 2: `canonical_name` ascending (stable, deterministic for tests).

### Cold-start note

Early on, everyone sits near 1500 and a few votes cause big reshuffles. That is
expected and self-corrects with volume. Showing `comparisons` next to each entry
(per [01](01-functional-spec.md)) sets the right expectation about confidence.

## Enhancements backlog (not v1)

- **K decay**: `K = 32` while `comparisons < 30`, then `16` — stabilizes
  established subjects while keeping newcomers nimble.
- **Glicko-2 / TrueSkill**: model rating *uncertainty* explicitly; better for
  sparse data, more complex. Revisit if Elo proves too jittery.
- **Provisional flag**: hide or mark subjects with `comparisons < N` on the
  board until they have enough data.
- **Recompute endpoint/CLI**: replay `votes` to rebuild ratings after a
  parameter change (enabled by the append-only `votes` log, see [03](03-data-model.md)).
