// Package model holds plain structs shared across the transport, service, and
// store layers (docs/03-data-model.md). No behavior — just data.
package model

import "time"

// Subject is the language-neutral core of a person in the pool, keyed by
// Wikidata QID.
type Subject struct {
	ID             int64
	WikidataID     string
	CanonicalName  string
	Source         string // "seed" | "user"
	AvailableLangs []string
	Rating         float64 // Glicko-2 rating (~1500 scale)
	RD             float64 // Glicko-2 rating deviation (uncertainty)
	Volatility     float64 // Glicko-2 volatility (σ)
	Wins           int
	Losses         int
	Comparisons    int
	GlobalViews    int64 // trailing-window Wikipedia pageviews summed across languages (global-fame lever, docs/10)
	Active         bool
	DiedAt         *time.Time // date of death (Wikidata P570); nil = living or unknown
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Translation is per-language display content, lazily cached from Wikipedia.
type Translation struct {
	SubjectID    int64
	Lang         string
	Name         string
	Description  string
	Extract      string // Wikipedia lead paragraph (shown inline on the matchup card)
	ImageURL     string
	WikipediaURL string
	FetchedAt    time.Time
}

// Vote is an append-only record of a single preference. The *Before fields
// snapshot each subject's full Glicko-2 state at vote time so ratings remain
// replayable from the log (docs/03-data-model.md §votes).
type Vote struct {
	ID                 int64
	SessionID          string
	WinnerID           int64
	LoserID            int64
	WinnerRatingBefore float64
	LoserRatingBefore  float64
	WinnerRDBefore     float64
	LoserRDBefore      float64
	WinnerVolBefore    float64
	LoserVolBefore     float64
	CreatedAt          time.Time
}

// Session is the anonymous, non-identifying browser counter plus the
// human-verified status (R12). Not authentication, not the leaderboard gate.
type Session struct {
	ID                 string
	Contributions      int
	HumanVerifiedUntil *time.Time
	CreatedAt          time.Time
	LastSeenAt         time.Time
}

// Stats is an aggregate, privacy-preserving snapshot of public activity
// (docs/04-api.md §GET /api/stats). Every field is a count over anonymous,
// non-identifying data — there are no per-visitor rows, no PII, no geography of
// visitors (their IP is never stored). It is safe to serve to anyone.
type Stats struct {
	Totals StatsTotals
	Daily  []DailyStat
}

// StatsTotals are the all-time headline counts.
type StatsTotals struct {
	Votes           int // preferences recorded, all-time
	Voters          int // distinct anonymous sessions that have ever voted
	Visitors        int // anonymous sessions ever created (≈ unique browsers/devices)
	Subjects        int // active people in the pool
	UserContributed int // subjects added by visitors (source='user')
}

// DailyStat is one UTC-day bucket of the activity time series. Days with no
// activity are present with zero counts (the series is gap-filled).
type DailyStat struct {
	Date     time.Time // UTC midnight of the day this bucket covers
	Votes    int       // votes cast that day
	Voters   int       // distinct sessions that voted that day
	Visitors int       // sessions first seen that day (≈ new visitors)
	Added    int       // people added to the pool that day
}

// SubjectPublic is the localized projection returned to clients
// (docs/04-api.md §Resource shapes). Ratings are omitted from matchups so the
// visitor isn't biased before choosing.
type SubjectPublic struct {
	ID           int64  `json:"id"`
	WikidataID   string `json:"wikidataId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Extract      string `json:"extract,omitempty"`
	ImageURL     string `json:"imageUrl,omitempty"`
	WikipediaURL string `json:"wikipediaUrl"`
	Deceased     bool   `json:"deceased,omitempty"` // has a recorded date of death (P570)
	DiedYear     int    `json:"diedYear,omitempty"` // year of death, for display next to the name
}
