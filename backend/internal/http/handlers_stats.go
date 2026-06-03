package http

import (
	"net/http"
	"time"
)

// --- GET /api/stats (public, ungated) ----------------------------------------
//
// Public transparency dashboard data. Unlike the leaderboard this is NOT gated:
// the figures are aggregate counts over anonymous data and reveal nothing about
// any individual visitor (docs/04-api.md §GET /api/stats). No session is
// minted — reading the stats shouldn't create a visitor.

type statsTotals struct {
	Votes           int `json:"votes"`
	Voters          int `json:"voters"`
	Visitors        int `json:"visitors"`
	Subjects        int `json:"subjects"`
	UserContributed int `json:"userContributed"`
}

type dailyStat struct {
	Date     string `json:"date"` // YYYY-MM-DD (UTC)
	Votes    int    `json:"votes"`
	Voters   int    `json:"voters"`
	Visitors int    `json:"visitors"`
	Added    int    `json:"added"`
}

type statsResponse struct {
	GeneratedAt string      `json:"generatedAt"`
	Days        int         `json:"days"`
	Totals      statsTotals `json:"totals"`
	Daily       []dailyStat `json:"daily"`
}

// stats serves the public activity snapshot: all-time totals plus a gap-filled
// daily time series for the trailing `days` days (default 30, 1–365).
func (h *handlers) stats(w http.ResponseWriter, r *http.Request) {
	days := clampInt(r.URL.Query().Get("days"), 30, 1, 365)

	st, err := h.store.Stats(r.Context(), days)
	if err != nil {
		h.logger.Error("stats", "err", err)
		writeError(w, http.StatusInternalServerError, "internal",
			"Could not load the stats.")
		return
	}

	daily := make([]dailyStat, 0, len(st.Daily))
	for _, d := range st.Daily {
		daily = append(daily, dailyStat{
			Date:     d.Date.Format("2006-01-02"),
			Votes:    d.Votes,
			Voters:   d.Voters,
			Visitors: d.Visitors,
			Added:    d.Added,
		})
	}

	writeJSON(w, http.StatusOK, statsResponse{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Days:        days,
		Totals: statsTotals{
			Votes:           st.Totals.Votes,
			Voters:          st.Totals.Voters,
			Visitors:        st.Totals.Visitors,
			Subjects:        st.Totals.Subjects,
			UserContributed: st.Totals.UserContributed,
		},
		Daily: daily,
	})
}
