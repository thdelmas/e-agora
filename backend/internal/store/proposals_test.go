package store

import "testing"

// TestPoolKey: the key is the geographic scope, fame/status filters don't change it.
func TestPoolKey(t *testing.T) {
	cases := []struct {
		name string
		pool Pool
		want string
	}{
		{"empty is world", Pool{}, "world"},
		{"continent", Pool{Continent: "Europe"}, "continent:Europe"},
		{"country", Pool{Country: "France"}, "country:France"},
		{"country is the finer scope, wins over continent", Pool{Country: "France", Continent: "Europe"}, "country:France"},
		{"fame/status don't change the key", Pool{Continent: "Africa", FameTop: true, IncludeDeceased: true}, "continent:Africa"},
		{"world ignores view filters", Pool{FameTop: true, IncludeDeceased: true}, "world"},
	}
	for _, c := range cases {
		if got := PoolKey(c.pool); got != c.want {
			t.Errorf("%s: PoolKey = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestBelongingScore pins the smoothing's defining properties (docs/11 §4): an
// empty pool sits at the neutral prior; a lone recall is heavily discounted (not
// ~1); confidence rises with evidence so a steady share converges upward; and a
// higher share always outscores a lower one at equal evidence.
func TestBelongingScore(t *testing.T) {
	// Empty pool → neutral prior π₀.
	if got := BelongingScore(0, 0); !approx(got, belongPriorShare, 1e-9) {
		t.Errorf("empty pool score = %v, want π₀ = %v", got, belongPriorShare)
	}

	// A single 1/1 recall must be far from certainty.
	if got := BelongingScore(1, 1); got > 0.1 {
		t.Errorf("1/1 score = %v, want heavily smoothed (≤0.1)", got)
	}

	// Same 100% share, more evidence → strictly higher confidence.
	if BelongingScore(100, 100) <= BelongingScore(1, 1) {
		t.Error("score should rise with evidence at equal share")
	}

	// Monotonic in share at equal evidence: 80/100 > 20/100.
	if BelongingScore(80, 100) <= BelongingScore(20, 100) {
		t.Error("score should increase with the recall share")
	}

	// A never-recalled subject in a busy pool sits below the prior (evidence of absence).
	if BelongingScore(0, 100) >= belongPriorShare {
		t.Errorf("0/100 score = %v, want below π₀", BelongingScore(0, 100))
	}
}

func approx(a, b, eps float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < eps
}
