package http

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/thdelmas/e-agora/backend/internal/model"
	"github.com/thdelmas/e-agora/backend/internal/store"
)

// membershipReason is the human-readable "why is this figure in this pool"
// shown beside the confirm/infirm control (docs/11 §7). Pool membership is
// seeded from Wikidata P27 (country of citizenship) → P30 (continent), so the
// reason names that basis. Empty for the world pool, which has no membership
// question — everyone belongs to the world.
func membershipReason(pool store.Pool) string {
	switch {
	case pool.Country != "":
		return "Citizen of " + pool.Country + " · Wikidata P27"
	case pool.Continent != "":
		return "From " + pool.Continent + " · Wikidata P27"
	default:
		return ""
	}
}

// verdictLabel / verdictValue translate between the stored signed verdict
// (+1 confirm, -1 infirm, 0 none) and the JSON string the client speaks.
func verdictLabel(v int) string {
	switch {
	case v > 0:
		return "confirm"
	case v < 0:
		return "infirm"
	default:
		return ""
	}
}

func verdictValue(s string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "confirm":
		return 1, true
	case "infirm":
		return -1, true
	case "", "none", "clear":
		return 0, true
	default:
		return 0, false
	}
}

// attachBelonging fills the Belonging projection on each subject for a
// geographic pool: the reason it was placed there plus the crowd's
// confirm/infirm tally, Beta confidence, and the viewer's own standing verdict
// (docs/11 §7). No-op for the world pool. Best-effort — a stats lookup failure
// is logged and leaves Belonging nil rather than failing the whole response.
func (h *handlers) attachBelonging(
	ctx context.Context, pool store.Pool, sessionID string,
	subs ...*model.SubjectPublic,
) {
	reason := membershipReason(pool)
	if reason == "" {
		return // world pool — no membership question
	}
	poolKey := store.PoolKey(pool)
	ids := make([]int64, 0, len(subs))
	for _, s := range subs {
		ids = append(ids, s.ID)
	}
	stats, err := h.store.MembershipFor(ctx, poolKey, sessionID, ids)
	if err != nil {
		h.logger.Warn("attach belonging", "err", err)
		return
	}
	for _, s := range subs {
		m := stats[s.ID] // zero value (prior) when no one has voted
		s.Belonging = &model.Belonging{
			PoolKey:       poolKey,
			Reason:        reason,
			Confirms:      m.Confirms,
			Infirms:       m.Infirms,
			Score:         m.Score,
			ViewerVerdict: verdictLabel(m.Verdict),
		}
	}
}

type membershipRequest struct {
	SubjectID int64  `json:"subjectId"`
	Verdict   string `json:"verdict"` // "confirm" | "infirm" | "none"
}

type membershipResponse struct {
	PoolKey       string  `json:"poolKey"`
	Confirms      int     `json:"confirms"`
	Infirms       int     `json:"infirms"`
	Score         float64 `json:"score"`
	ViewerVerdict string  `json:"viewerVerdict,omitempty"`
	Excluded      bool    `json:"excluded"`
}

// membership records the session's confirm/infirm verdict on a subject's
// membership in the active pool (docs/11 §7) and returns the refreshed tally.
// Like a proposal it's rate-limited but not humanity-gated — it's a low-stakes
// membership signal, and the Beta prior plus the recount-from-votes design mean
// a single session can't move much. The pool scope comes from the same query
// params as the matchup (poolFrom → PoolKey); the world pool is rejected.
func (h *handlers) membership(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())

	if h.cfg.RateLimitOn {
		if ok, retry := h.limiter.Allow(sess.ID); !ok {
			w.Header().Set("Retry-After",
				strconv.Itoa(int(math.Ceil(retry.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "rate_limited",
				"Whoa — slow down a moment.")
			return
		}
	}

	var req membershipRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"Malformed request body.")
		return
	}
	if req.SubjectID == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"Provide a subjectId.")
		return
	}
	verdict, ok := verdictValue(req.Verdict)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"verdict must be confirm, infirm or none.")
		return
	}

	poolKey := store.PoolKey(h.poolFrom(r))
	stat, err := h.store.RecordMembershipVote(
		r.Context(), sess.ID, poolKey, req.SubjectID, verdict)
	switch {
	case errors.Is(err, store.ErrMembershipScope):
		writeError(w, http.StatusBadRequest, "no_pool",
			"Pick a country or region before judging membership.")
		return
	case errors.Is(err, store.ErrInvalidProposal):
		writeError(w, http.StatusBadRequest, "invalid_subject",
			"That isn't an active subject.")
		return
	case err != nil:
		h.logger.Error("membership: record", "err", err)
		writeError(w, http.StatusInternalServerError, "internal",
			"Could not record your verdict.")
		return
	}

	writeJSON(w, http.StatusOK, membershipResponse{
		PoolKey:       poolKey,
		Confirms:      stat.Confirms,
		Infirms:       stat.Infirms,
		Score:         stat.Score,
		ViewerVerdict: verdictLabel(stat.Verdict),
		Excluded:      store.MembershipExcluded(stat.Confirms, stat.Infirms),
	})
}

type subjectPool struct {
	PoolKey       string  `json:"poolKey"`
	Scope         string  `json:"scope"` // "country" | "continent"
	Label         string  `json:"label"`
	Reason        string  `json:"reason"`
	Confirms      int     `json:"confirms"`
	Infirms       int     `json:"infirms"`
	Score         float64 `json:"score"`
	ViewerVerdict string  `json:"viewerVerdict,omitempty"`
	Excluded      bool    `json:"excluded"`
}

// subjectPools lists every geographic pool a subject is in (one per citizenship
// and per continent those span, docs/10 §4), each with its why-reason and the
// crowd's membership verdict — the data behind the per-pool confirm/infirm list
// on the subject detail view (docs/11 §7).
func (h *handlers) subjectPools(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"Bad subject id.")
		return
	}
	countries, continents, found, err := h.store.SubjectGeo(r.Context(), id)
	if err != nil {
		h.logger.Error("subject pools: geo", "err", err)
		writeError(w, http.StatusInternalServerError, "internal",
			"Could not load pools.")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not_found", "No such subject.")
		return
	}

	pools := make([]subjectPool, 0, len(countries)+len(continents))
	add := func(scope, label string, pool store.Pool) {
		poolKey := store.PoolKey(pool)
		stats, err := h.store.MembershipFor(
			r.Context(), poolKey, sess.ID, []int64{id})
		if err != nil {
			h.logger.Warn("subject pools: membership", "err", err)
			return
		}
		m := stats[id]
		pools = append(pools, subjectPool{
			PoolKey: poolKey, Scope: scope, Label: label,
			Reason:        membershipReason(pool),
			Confirms:      m.Confirms,
			Infirms:       m.Infirms,
			Score:         m.Score,
			ViewerVerdict: verdictLabel(m.Verdict),
			Excluded:      store.MembershipExcluded(m.Confirms, m.Infirms),
		})
	}
	for _, c := range countries {
		add("country", c, store.Pool{Country: c})
	}
	for _, c := range continents {
		add("continent", c, store.Pool{Continent: c})
	}
	writeJSON(w, http.StatusOK, map[string]any{"pools": pools})
}
