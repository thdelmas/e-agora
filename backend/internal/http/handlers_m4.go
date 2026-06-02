package http

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/thdelmas/e-agora/backend/internal/lang"
	"github.com/thdelmas/e-agora/backend/internal/model"
	"github.com/thdelmas/e-agora/backend/internal/store"
	"github.com/thdelmas/e-agora/backend/internal/subjects"
	"github.com/thdelmas/e-agora/backend/internal/token"
)

// --- GET /api/me -------------------------------------------------------------

type meResponse struct {
	Contributions      int    `json:"contributions"`
	HasAccess          bool   `json:"hasAccess"`
	AccessExpiresAt    string `json:"accessExpiresAt,omitempty"`
	CanAdd             bool   `json:"canAdd"`
	HumanVerified      bool   `json:"humanVerified"`
	HumanVerifiedUntil string `json:"humanVerifiedUntil,omitempty"`
}

// me reports session/access state to drive the client UI.
func (h *handlers) me(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	resp := meResponse{Contributions: sess.Contributions}

	if sess.HumanVerifiedUntil != nil && sess.HumanVerifiedUntil.After(time.Now()) {
		resp.HumanVerified = true
		resp.HumanVerifiedUntil = sess.HumanVerifiedUntil.UTC().Format(time.RFC3339)
	}
	if c, err := r.Cookie(accessCookie); err == nil {
		if claims, err := token.Verify(h.cfg.TokenSecret, c.Value); err == nil {
			resp.HasAccess = true
			resp.AccessExpiresAt = time.Unix(claims.Exp, 0).UTC().Format(time.RFC3339)
			if used, err := h.store.AddTokenUsed(r.Context(), claims.Jti); err == nil {
				resp.CanAdd = !used
			}
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// --- GET /api/leaderboard (gated, R4+R10) ------------------------------------

type leaderboardEntry struct {
	Rank            int                 `json:"rank"`
	Subject         model.SubjectPublic `json:"subject"`
	Rating          float64             `json:"rating"`
	RatingDeviation float64             `json:"ratingDeviation"` // Glicko-2 RD; high = provisional
	Wins            int                 `json:"wins"`
	Losses          int                 `json:"losses"`
	Comparisons     int                 `json:"comparisons"`
	Lang            string              `json:"lang"`
}

type leaderboardResponse struct {
	TotalVotes int                `json:"totalVotes"`
	Limit      int                `json:"limit"`
	Offset     int                `json:"offset"`
	Count      int                `json:"count"`
	Entries    []leaderboardEntry `json:"entries"`
}

func (h *handlers) leaderboard(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireAccess(w, r); !ok {
		return
	}
	limit := clampInt(r.URL.Query().Get("limit"), 100, 1, 500)
	offset := clampInt(r.URL.Query().Get("offset"), 0, 0, 1_000_000)

	subs, err := h.store.TopByRating(r.Context(), limit, offset, h.poolFrom(r))
	if err != nil {
		h.logger.Error("leaderboard", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Could not load the rankings.")
		return
	}
	total, err := h.store.TotalVotes(r.Context())
	if err != nil {
		h.logger.Error("total votes", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Could not load the rankings.")
		return
	}

	display := lang.Pick(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"), h.cfg.FallbackLang)
	entries := make([]leaderboardEntry, 0, len(subs))
	for i, s := range subs {
		tr := h.localize(r.Context(), s, display)
		entries = append(entries, leaderboardEntry{
			Rank:            offset + i + 1,
			Subject:         publicOf(s, tr),
			Rating:          math.Round(s.Rating*10) / 10,
			RatingDeviation: math.Round(s.RD*10) / 10,
			Wins:            s.Wins,
			Losses:          s.Losses,
			Comparisons:     s.Comparisons,
			Lang:            tr.Lang,
		})
	}
	writeJSON(w, http.StatusOK, leaderboardResponse{
		TotalVotes: total, Limit: limit, Offset: offset, Count: len(entries), Entries: entries,
	})
}

// requireAccess enforces the leaderboard/add gate: a valid, non-expired access
// token. It writes the 403 and returns ok=false on failure.
func (h *handlers) requireAccess(w http.ResponseWriter, r *http.Request) (token.Claims, bool) {
	c, err := r.Cookie(accessCookie)
	if err != nil {
		writeError(w, http.StatusForbidden, "access_required", "Vote once to unlock the rankings for 24 hours.")
		return token.Claims{}, false
	}
	claims, err := token.Verify(h.cfg.TokenSecret, c.Value)
	if errors.Is(err, token.ErrExpired) {
		writeError(w, http.StatusForbidden, "access_expired", "Your 24-hour access has expired — vote again to unlock.")
		return token.Claims{}, false
	}
	if err != nil {
		writeError(w, http.StatusForbidden, "access_required", "Vote once to unlock the rankings for 24 hours.")
		return token.Claims{}, false
	}
	return claims, true
}

// --- POST /api/subjects (R8/R8.1) --------------------------------------------

type addSubjectRequest struct {
	URL        string `json:"url"`
	WikidataID string `json:"wikidataId"`
}

func (h *handlers) addSubject(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	claims, ok := h.requireAccess(w, r)
	if !ok {
		return
	}
	if h.cfg.RateLimitOn {
		if allowed, retry := h.limiter.Allow(sess.ID); !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Whoa — slow down a moment.")
			return
		}
	}

	var req addSubjectRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Malformed request body.")
		return
	}

	out, err := h.addsvc.Add(r.Context(),
		subjects.AddInput{URL: req.URL, WikidataID: req.WikidataID},
		claims.Jti, time.Unix(claims.Exp, 0))
	switch {
	case errors.Is(err, subjects.ErrNotPerson):
		writeError(w, http.StatusUnprocessableEntity, "not_a_person", "e-agora is for people — that page isn't a person.")
	case errors.Is(err, subjects.ErrNoPage), errors.Is(err, subjects.ErrBadInput):
		writeError(w, http.StatusUnprocessableEntity, "not_a_wikipedia_page", "We couldn't find a Wikipedia page for that.")
	case errors.Is(err, subjects.ErrExists):
		writeError(w, http.StatusConflict, "already_exists", "They're already in the agora.")
	case errors.Is(err, subjects.ErrAddLimit):
		writeError(w, http.StatusTooManyRequests, "add_limit_reached", "You can add one person per 24 hours — vote again after your access renews.")
	case err != nil:
		h.logger.Error("add subject", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Could not add that subject.")
	default:
		writeJSON(w, http.StatusCreated, map[string]any{"subject": out})
	}
}

// --- GET /api/subjects/search ------------------------------------------------

func (h *handlers) subjectsSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "q is required.")
		return
	}
	l := lang.Pick(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"), h.cfg.FallbackLang)
	results, err := h.ingest.Search(r.Context(), l, q, 8)
	if err != nil {
		h.logger.Error("subjects search", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Search is unavailable right now.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// poolFrom reads the visitor's pool selection (docs/10 §4) from the query — the
// scope shared by the matchup draw and the leaderboard view: status
// (?includeDeceased), region (?region=Europe), and fame tier (?fameTier=top).
// An empty selection is the whole living pool (the prior default).
func (h *handlers) poolFrom(r *http.Request) store.Pool {
	q := r.URL.Query()
	return store.Pool{
		IncludeDeceased: boolParam(q.Get("includeDeceased")),
		Continent:       strings.TrimSpace(q.Get("region")),
		FameTop:         strings.EqualFold(q.Get("fameTier"), "top"),
		FamePct:         h.cfg.FameTierPct,
	}
}

// boolParam parses a query flag: "1", "true" or "yes" (any case) are true; an
// absent or any other value is false (so the deceased filter defaults to hiding
// them).
func boolParam(raw string) bool {
	switch strings.ToLower(raw) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// clampInt parses a query int, applying a default and [min,max] bounds.
func clampInt(raw string, def, lo, hi int) int {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}
