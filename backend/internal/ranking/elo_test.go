package ranking

import (
	"math"
	"testing"
)

const eps = 1e-9

// An even matchup moves exactly ±16 at K=32 (docs/05-ranking.md worked example).
func TestUpdate_EvenMatchupMoves16(t *testing.T) {
	w, l := Update(1500, 1500)
	if math.Abs(w-1516) > eps {
		t.Errorf("winner = %v, want 1516", w)
	}
	if math.Abs(l-1484) > eps {
		t.Errorf("loser = %v, want 1484", l)
	}
}

// Total rating is conserved: the winner gains what the loser loses.
func TestUpdate_Conservation(t *testing.T) {
	for _, tc := range []struct{ rw, rl float64 }{
		{1500, 1500}, {1300, 1700}, {1700, 1300}, {1234.5, 1888.2},
	} {
		w, l := Update(tc.rw, tc.rl)
		gained := w - tc.rw
		lost := tc.rl - l
		if math.Abs(gained-lost) > eps {
			t.Errorf("Update(%v,%v): winner gained %v but loser lost %v", tc.rw, tc.rl, gained, lost)
		}
	}
}

// A win never decreases the winner; a loss never increases the loser.
func TestUpdate_Monotonicity(t *testing.T) {
	w, l := Update(1300, 1700)
	if w <= 1300 {
		t.Errorf("winner rating did not increase: %v", w)
	}
	if l >= 1700 {
		t.Errorf("loser rating did not decrease: %v", l)
	}
}

// An upset (underdog wins) swings more than an expected result.
func TestUpdate_UpsetSwingsMore(t *testing.T) {
	upsetW, _ := Update(1300, 1700)   // underdog wins
	expectW, _ := Update(1700, 1300)  // favorite wins
	upsetGain := upsetW - 1300
	expectGain := expectW - 1700
	if upsetGain <= expectGain {
		t.Errorf("upset gain %v should exceed expected-win gain %v", upsetGain, expectGain)
	}
}
