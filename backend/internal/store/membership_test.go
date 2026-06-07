package store

import "testing"

// TestMembershipConfidence pins the Beta posterior's defining properties
// (docs/11 §7): an unvoted membership sits at the trusting prior (0.8, not
// 0.5), infirms pull it down, confirms push it up, and confirms recover ground
// that infirms took.
func TestMembershipConfidence(t *testing.T) {
	prior := MembershipConfidence(0, 0)
	if prior < 0.79 || prior > 0.81 {
		t.Errorf("prior confidence = %.3f, want ~0.8 (trust Wikidata)", prior)
	}
	if down := MembershipConfidence(0, 3); down >= prior {
		t.Errorf("3 infirms = %.3f, want below prior %.3f", down, prior)
	}
	if up := MembershipConfidence(3, 0); up <= prior {
		t.Errorf("3 confirms = %.3f, want above prior %.3f", up, prior)
	}
	// Confirms offset infirms: an even split lands between the two one-sided
	// extremes, above the all-infirm case.
	even := MembershipConfidence(3, 3)
	if even <= MembershipConfidence(0, 6) {
		t.Errorf("balanced 3/3 = %.3f should beat 0/6 = %.3f",
			even, MembershipConfidence(0, 6))
	}
}

// TestMembershipGate: the gate never boosts (caps at 1 at/above the prior) and
// slides below 1 only as the crowd argues a figure out.
func TestMembershipGate(t *testing.T) {
	if g := MembershipGate(0, 0); g != 1 {
		t.Errorf("unvoted gate = %.3f, want 1 (no change)", g)
	}
	if g := MembershipGate(50, 0); g != 1 {
		t.Errorf("heavily-confirmed gate = %.3f, want capped at 1", g)
	}
	g1, g5 := MembershipGate(0, 1), MembershipGate(0, 5)
	if !(g5 < g1 && g1 < 1) {
		t.Errorf("gate should shrink with infirms: 1→%.3f, 5→%.3f", g1, g5)
	}
}

// TestMembershipExcluded: a couple of dissents don't override Wikidata, but a
// sustained infirm run drops the figure from the pool — and a confirm can pull
// them back above the line.
func TestMembershipExcluded(t *testing.T) {
	if MembershipExcluded(0, 2) {
		t.Error("2 infirms should NOT hard-exclude (too little consensus)")
	}
	if !MembershipExcluded(0, 40) {
		t.Error("40 infirms should hard-exclude")
	}
	// Find the smallest infirm count that excludes, then show one confirm at the
	// same infirm level moves the needle (confidence strictly rises).
	n := 0
	for !MembershipExcluded(0, n) {
		n++
	}
	if MembershipConfidence(1, n) <= MembershipConfidence(0, n) {
		t.Error("a confirm must raise confidence at the same infirm level")
	}
}
