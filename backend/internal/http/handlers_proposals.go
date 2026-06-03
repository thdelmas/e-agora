package http

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/thdelmas/e-agora/backend/internal/store"
	"github.com/thdelmas/e-agora/backend/internal/subjects"
)

// recall searches active subjects by name for the recall type-ahead (docs/11 §2).
// Public-with-session like the matchup; an empty q returns nothing.
func (h *handlers) recall(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"results": []store.SubjectRef{}})
		return
	}
	results, err := h.store.RecallSearch(r.Context(), q, 8)
	if err != nil {
		h.logger.Error("recall search", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Search is unavailable right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// proposalRequest names the recalled figure: subjectId for someone already in the
// pool, or url (a Wikipedia URL) for someone not yet seeded — the latter is
// ingested on first recall (docs/11 §3).
type proposalRequest struct {
	SubjectID int64  `json:"subjectId"`
	URL       string `json:"url"`
}

type proposalResponse struct {
	PoolKey string `json:"poolKey"`
}

// proposal records one recall of a subject for the active pool (docs/11 §2) — the
// belonging signal. Rate-limited but, unlike a vote, *not* humanity-gated: recall
// is the visitor's first interaction (pool entry, before any vote), so gating it
// would wall off the casual visitor the recognition redesign exists to serve. The
// abuse surface is low — the smoothed score moves n and N together, so one session
// is near-neutral and shifting belonging needs many distinct sessions (which the
// humanity check on *votes* still discourages). The pool scope comes from the same
// query params as the matchup/leaderboard (poolFrom → PoolKey); subject in the body.
func (h *handlers) proposal(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())

	if h.cfg.RateLimitOn {
		if ok, retry := h.limiter.Allow(sess.ID); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Whoa — slow down a moment.")
			return
		}
	}

	var req proposalRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}

	subjectID := req.SubjectID
	if subjectID == 0 {
		// A figure not yet in the pool: resolve the Wikipedia URL and ingest them
		// on first recall (docs/11 §3), reusing the add path's human + page checks.
		if strings.TrimSpace(req.URL) == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "Provide a subjectId or url to propose.")
			return
		}
		id, _, err := h.addsvc.EnsureFromURL(r.Context(), req.URL)
		switch {
		case errors.Is(err, subjects.ErrNotPerson):
			writeError(w, http.StatusUnprocessableEntity, "not_a_person", "e-agora is for people — that page isn't a person.")
			return
		case errors.Is(err, subjects.ErrNoPage), errors.Is(err, subjects.ErrBadInput):
			writeError(w, http.StatusUnprocessableEntity, "not_a_wikipedia_page", "We couldn't find a Wikipedia page for that.")
			return
		case err != nil:
			h.logger.Error("proposal: ensure from url", "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "Could not add that person.")
			return
		}
		subjectID = id
	}

	poolKey := store.PoolKey(h.poolFrom(r))
	err := h.store.RecordProposal(r.Context(), sess.ID, poolKey, subjectID)
	if errors.Is(err, store.ErrInvalidProposal) {
		writeError(w, http.StatusBadRequest, "invalid_proposal", "That isn't an active subject.")
		return
	}
	if err != nil {
		h.logger.Error("proposal: record", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Could not record your proposal.")
		return
	}
	writeJSON(w, http.StatusOK, proposalResponse{PoolKey: poolKey})
}
