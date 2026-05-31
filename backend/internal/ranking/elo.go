// Package ranking implements the Elo rating update applied on every vote
// (docs/05-ranking.md). It is a pure function over two ratings — no I/O — so it
// is trivially unit-testable and called inside the vote transaction.
package ranking

import "math"

const (
	// DefaultRating is where every new subject starts.
	DefaultRating = 1500.0
	// KFactor is the Elo update step size (v1 default).
	KFactor = 32.0
	// scale is the standard Elo logistic scale.
	scale = 400.0
)

// expected returns the expected score of A against B (probability A wins).
func expected(rA, rB float64) float64 {
	return 1.0 / (1.0 + math.Pow(10, (rB-rA)/scale))
}

// Update returns the winner's and loser's new ratings after W beats L.
// Total rating is conserved: the winner gains exactly what the loser drops.
func Update(rWinner, rLoser float64) (newWinner, newLoser float64) {
	eW := expected(rWinner, rLoser)
	newWinner = rWinner + KFactor*(1.0-eW)
	newLoser = rLoser + KFactor*(0.0-(1.0-eW))
	return
}
