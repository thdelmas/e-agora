// Package ranking implements the Glicko-2 rating update applied on every vote
// (docs/05-ranking.md). Each vote is treated as a one-game rating period for
// both subjects: the winner and loser are each updated against the other's
// pre-vote state. It is a pure function over two ratings — no I/O — so it
// is trivially unit-testable and called inside the vote transaction.
//
// Glicko-2 tracks three numbers per subject instead of Elo's one:
//   - R   the rating (same ~1500 display scale as Elo);
//   - RD  the rating deviation — how unsure we are of R (shrinks with
//     evidence);
//   - σ   the volatility — how erratic the subject's results have been.
//
// RD is the payoff: an unproven subject (high RD) moves fast and is ranked
// conservatively, while a settled one (low RD) barely twitches. Unlike Elo,
// total rating is NOT conserved — the winner's gain need not equal the
// loser's loss, because each moves in proportion to its own uncertainty.
package ranking

import "math"

const (
	// DefaultRating is where every new subject starts (display scale).
	DefaultRating = 1500.0
	// DefaultDeviation is a new subject's rating deviation — maximally unsure.
	DefaultDeviation = 350.0
	// DefaultVolatility is a new subject's volatility (Glickman's default).
	DefaultVolatility = 0.06

	// tau constrains how much volatility can change per period. Smaller =
	// steadier ratings; Glickman suggests 0.3–1.2. 0.5 suits our noisy,
	// opinion-based votes.
	tau = 0.5
	// glickoScale converts between the display scale (R, RD) and Glicko-2's
	// internal scale (μ, φ): μ = (R − 1500)/glickoScale, φ = RD/glickoScale.
	glickoScale = 173.7178
	// convergence tolerance for the volatility root-finding iteration.
	epsilon = 1e-6
)

// Rating is a subject's full Glicko-2 state.
type Rating struct {
	R   float64 // rating, display scale (~1500)
	RD  float64 // rating deviation (uncertainty)
	Vol float64 // volatility (σ)
}

// Update returns the winner's and loser's new Glicko-2 states after W beats L.
// Each is updated against the other's pre-vote state (one game, score 1 vs 0).
func Update(winner, loser Rating) (newWinner, newLoser Rating) {
	newWinner = apply(winner, []game{gameFrom(loser, 1)})
	newLoser = apply(loser, []game{gameFrom(winner, 0)})
	return
}

// game is one opponent in a rating period, on Glicko-2's internal scale.
type game struct {
	muJ, phiJ, score float64
}

func gameFrom(opp Rating, score float64) game {
	return game{
		muJ:   (opp.R - DefaultRating) / glickoScale,
		phiJ:  opp.RD / glickoScale,
		score: score,
	}
}

// apply runs one Glicko-2 rating period for subject p against the given games,
// following Glickman's "Example of the Glicko-2 system" step for step. With a
// single game this is the per-vote update; the slice form keeps the algorithm
// faithful to the paper (and unit-testable against its worked example).
func apply(p Rating, games []game) Rating {
	// Step 2: to the internal scale.
	mu := (p.R - DefaultRating) / glickoScale
	phi := p.RD / glickoScale

	// Step 3 & 4: estimated variance v and the rating-direction quantity Σ.
	var invV, sum float64
	for _, gm := range games {
		gj := g(gm.phiJ)
		e := expected(mu, gm.muJ, gm.phiJ)
		invV += gj * gj * e * (1 - e)
		sum += gj * (gm.score - e)
	}
	v := 1.0 / invV
	delta := v * sum

	// Step 5: new volatility (the iterative bit).
	volNew := newVolatility(phi, p.Vol, v, delta)

	// Step 6: inflate RD by the new volatility (the only inflation we model — we
	// do not age RD between votes, since a subject's appeal is ~stationary).
	phiStar := math.Sqrt(phi*phi + volNew*volNew)

	// Step 7: new RD and rating from the game evidence.
	phiNew := 1.0 / math.Sqrt(1.0/(phiStar*phiStar)+1.0/v)
	muNew := mu + phiNew*phiNew*sum

	// Step 8: back to the display scale.
	return Rating{
		R:   DefaultRating + glickoScale*muNew,
		RD:  glickoScale * phiNew,
		Vol: volNew,
	}
}

// g weights an opponent's contribution by their certainty: a sure opponent
// (low φ) counts for more than an unproven one.
func g(phi float64) float64 {
	return 1.0 / math.Sqrt(1.0+3.0*phi*phi/(math.Pi*math.Pi))
}

// expected is the probability that μ beats an opponent (μ_j, φ_j).
func expected(mu, muJ, phiJ float64) float64 {
	return 1.0 / (1.0 + math.Exp(-g(phiJ)*(mu-muJ)))
}

// newVolatility solves Glickman's volatility equation for σ' via the Illinois
// algorithm (regula falsi), a bracketed root finder that always converges.
func newVolatility(phi, vol, v, delta float64) float64 {
	a := math.Log(vol * vol)
	f := func(x float64) float64 {
		ex := math.Exp(x)
		d := phi*phi + v + ex
		return ex*(delta*delta-d)/(2*d*d) - (x-a)/(tau*tau)
	}

	// Initial bracket [A, B] with f(A) and f(B) of opposite sign.
	A := a
	var B float64
	if d2 := delta * delta; d2 > phi*phi+v {
		B = math.Log(d2 - phi*phi - v)
	} else {
		k := 1.0
		for f(a-k*tau) < 0 {
			k++
		}
		B = a - k*tau
	}

	fA, fB := f(A), f(B)
	for math.Abs(B-A) > epsilon {
		C := A + (A-B)*fA/(fB-fA)
		fC := f(C)
		if fC*fB <= 0 {
			A, fA = B, fB
		} else {
			fA /= 2
		}
		B, fB = C, fC
	}
	return math.Exp(A / 2)
}
