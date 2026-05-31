package ranking

import (
	"math"
	"testing"
)

// The gold standard: Glickman's own worked example ("Example of the Glicko-2
// system"). A player at 1500/200/0.06 plays three games (win, loss, loss) in one
// rating period and must end at 1464.06 / 151.52 / 0.05999. Reproducing it pins
// the whole algorithm — g, E, variance, the volatility solver, and the scale
// conversions — to a published reference.
func TestApply_GlickmanWorkedExample(t *testing.T) {
	player := Rating{R: 1500, RD: 200, Vol: 0.06}
	games := []game{
		gameFrom(Rating{R: 1400, RD: 30}, 1),
		gameFrom(Rating{R: 1550, RD: 100}, 0),
		gameFrom(Rating{R: 1700, RD: 300}, 0),
	}
	got := apply(player, games)

	if math.Abs(got.R-1464.06) > 0.01 {
		t.Errorf("R = %.4f, want 1464.06", got.R)
	}
	if math.Abs(got.RD-151.52) > 0.01 {
		t.Errorf("RD = %.4f, want 151.52", got.RD)
	}
	if math.Abs(got.Vol-0.05999) > 1e-5 {
		t.Errorf("Vol = %.6f, want 0.05999", got.Vol)
	}
}

func newSubject() Rating {
	return Rating{R: DefaultRating, RD: DefaultDeviation, Vol: DefaultVolatility}
}

// A win raises the winner and lowers the loser; deviation shrinks for both as
// they gain evidence.
func TestUpdate_Direction(t *testing.T) {
	w, l := Update(newSubject(), newSubject())
	if w.R <= DefaultRating {
		t.Errorf("winner rating did not increase: %.2f", w.R)
	}
	if l.R >= DefaultRating {
		t.Errorf("loser rating did not decrease: %.2f", l.R)
	}
	if w.RD >= DefaultDeviation || l.RD >= DefaultDeviation {
		t.Errorf("deviation should shrink with a played game: w=%.2f l=%.2f", w.RD, l.RD)
	}
}

// An even matchup is symmetric: the winner gains exactly what the loser loses,
// and both land at the same deviation/volatility.
func TestUpdate_EvenMatchupSymmetric(t *testing.T) {
	w, l := Update(newSubject(), newSubject())
	if math.Abs((w.R-DefaultRating)-(DefaultRating-l.R)) > 1e-9 {
		t.Errorf("even matchup not symmetric: w=%.4f l=%.4f", w.R, l.R)
	}
	if math.Abs(w.RD-l.RD) > 1e-9 || math.Abs(w.Vol-l.Vol) > 1e-12 {
		t.Errorf("even matchup deviation/volatility differ: %+v %+v", w, l)
	}
}

// Glicko-2 is NOT zero-sum (the property that distinguishes it from Elo): a
// settled favorite (low RD) barely moves, while an unproven underdog (high RD)
// swings hard on the same result, so total rating is not conserved.
func TestUpdate_NotConservedUncertaintyDrivesMagnitude(t *testing.T) {
	settled := Rating{R: 1700, RD: 40, Vol: 0.06}   // proven favorite
	unproven := Rating{R: 1700, RD: 350, Vol: 0.06} // freshly added, same rating
	opponent := Rating{R: 1500, RD: 80, Vol: 0.06}

	// Same result (the 1700 loses to the 1500) from each starting point.
	_, settledAfter := Update(opponent, settled)
	_, unprovenAfter := Update(opponent, unproven)

	settledDrop := settled.R - settledAfter.R
	unprovenDrop := unproven.R - unprovenAfter.R
	if unprovenDrop <= settledDrop {
		t.Errorf("unproven subject should swing more (%.2f) than settled (%.2f)", unprovenDrop, settledDrop)
	}

	// And the same game is not zero-sum: the 1500's gain need not match the drop.
	winnerAfter, loserAfter := Update(opponent, unproven)
	gain := winnerAfter.R - opponent.R
	loss := unproven.R - loserAfter.R
	if math.Abs(gain-loss) < 1e-6 {
		t.Errorf("expected non-conservation, but gain %.4f == loss %.4f", gain, loss)
	}
}

// An upset (underdog beats favorite) moves the underdog more than an expected
// win would, for equally-certain subjects.
func TestUpdate_UpsetSwingsMore(t *testing.T) {
	rd := 100.0
	upsetWinner, _ := Update(Rating{R: 1300, RD: rd, Vol: 0.06}, Rating{R: 1700, RD: rd, Vol: 0.06})
	expectWinner, _ := Update(Rating{R: 1700, RD: rd, Vol: 0.06}, Rating{R: 1300, RD: rd, Vol: 0.06})
	if (upsetWinner.R - 1300) <= (expectWinner.R - 1700) {
		t.Errorf("upset gain %.2f should exceed expected-win gain %.2f", upsetWinner.R-1300, expectWinner.R-1700)
	}
}

// Deviation is monotone in evidence: replaying even matchups drives RD down and
// keeps it positive, never below zero or diverging.
func TestUpdate_DeviationConvergesDownward(t *testing.T) {
	s := newSubject()
	prev := s.RD
	for i := 0; i < 30; i++ {
		s, _ = Update(s, Rating{R: 1500, RD: 60, Vol: 0.06})
		if s.RD <= 0 {
			t.Fatalf("RD went non-positive: %.4f", s.RD)
		}
		if s.RD > prev+1e-9 {
			t.Fatalf("RD increased on a played game: %.4f -> %.4f", prev, s.RD)
		}
		prev = s.RD
	}
	if prev > 120 {
		t.Errorf("RD failed to tighten after 30 games: %.2f", prev)
	}
}
