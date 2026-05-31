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
	Country        string
	Source         string // "seed" | "user"
	AvailableLangs []string
	Rating         float64 // Glicko-2 rating (~1500 scale)
	RD             float64 // Glicko-2 rating deviation (uncertainty)
	Volatility     float64 // Glicko-2 volatility (σ)
	Wins           int
	Losses         int
	Comparisons    int
	Active         bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Translation is per-language display content, lazily cached from Wikipedia.
type Translation struct {
	SubjectID    int64
	Lang         string
	Name         string
	Description  string
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

// SubjectPublic is the localized projection returned to clients
// (docs/04-api.md §Resource shapes). Ratings are omitted from matchups so the
// visitor isn't biased before choosing.
type SubjectPublic struct {
	ID           int64  `json:"id"`
	WikidataID   string `json:"wikidataId"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Country      string `json:"country,omitempty"`
	ImageURL     string `json:"imageUrl,omitempty"`
	WikipediaURL string `json:"wikipediaUrl"`
}
