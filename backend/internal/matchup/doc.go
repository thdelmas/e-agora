// Package matchup selects a pair of distinct active subjects to compare
// (docs/05-ranking.md §Matchup pairing): uniform random in v1, coverage-biased
// (favoring under-compared subjects) in v1.1.
//
// Implemented in M3 (docs/07-roadmap.md).
package matchup
