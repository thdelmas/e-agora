package http

import (
	"context"
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
	"github.com/thdelmas/e-agora/backend/internal/token"
)

// --- matchup -----------------------------------------------------------------

type matchupResponse struct {
	A               model.SubjectPublic `json:"a"`
	B               model.SubjectPublic `json:"b"`
	DisplayLang     string              `json:"displayLang"`
	FallbackApplied bool                `json:"fallbackApplied"`
}

// matchup returns two distinct active subjects, localized per R9. By default the
// pair is drawn from the living; ?includeDeceased opts historical figures back in
// so a visitor can compare them against the living (docs/05-ranking.md §Filtering
// the deceased).
func (h *handlers) matchup(w http.ResponseWriter, r *http.Request) {
	includeDeceased := boolParam(r.URL.Query().Get("includeDeceased"))
	pair, err := h.store.RandomPair(r.Context(), includeDeceased)
	if errors.Is(err, store.ErrPoolTooSmall) {
		writeError(w, http.StatusConflict, "pool_too_small", "The agora is still being set up — check back soon.")
		return
	}
	if err != nil {
		h.logger.Error("matchup: random pair", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Could not load a matchup.")
		return
	}
	a, b := pair[0], pair[1]

	visitor := lang.Pick(r.URL.Query().Get("lang"), r.Header.Get("Accept-Language"), h.cfg.FallbackLang)
	display, fellBack := lang.Resolve(visitor, h.cfg.FallbackLang, a.AvailableLangs, b.AvailableLangs)

	ta := h.ensureExtract(r.Context(), a, h.localize(r.Context(), a, display))
	tb := h.ensureExtract(r.Context(), b, h.localize(r.Context(), b, display))
	writeJSON(w, http.StatusOK, matchupResponse{
		A:               publicOf(a, ta),
		B:               publicOf(b, tb),
		DisplayLang:     display,
		FallbackApplied: fellBack,
	})
}

// --- vote --------------------------------------------------------------------

type voteRequest struct {
	WinnerID int64 `json:"winnerId"`
	LoserID  int64 `json:"loserId"`
}

type voteResponse struct {
	Contributions        int    `json:"contributions"`
	AccessTokenExpiresAt string `json:"accessTokenExpiresAt"`
}

// vote records a preference (R5/R6). Gated by the rate limit (R11) and the
// humanity check (R12); mints a 24h access token when none is valid (R10).
func (h *handlers) vote(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())

	if h.cfg.RateLimitOn {
		if ok, retry := h.limiter.Allow(sess.ID); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retry.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "Whoa — slow down a moment.")
			return
		}
	}

	if sess.HumanVerifiedUntil == nil || !sess.HumanVerifiedUntil.After(time.Now()) {
		writeError(w, http.StatusForbidden, "human_check_required", "Prove you're human before voting.")
		return
	}

	var req voteRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.WinnerID == 0 || req.LoserID == 0 || req.WinnerID == req.LoserID {
		writeError(w, http.StatusBadRequest, "invalid_matchup", "Provide distinct winnerId and loserId.")
		return
	}

	res, err := h.store.RecordVote(r.Context(), sess.ID, req.WinnerID, req.LoserID)
	if errors.Is(err, store.ErrInvalidMatchup) {
		writeError(w, http.StatusBadRequest, "invalid_matchup", "Those subjects aren't a valid, active pair.")
		return
	}
	if err != nil {
		h.logger.Error("vote: record", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "Could not record your vote.")
		return
	}

	exp := h.ensureAccessToken(w, r)
	writeJSON(w, http.StatusOK, voteResponse{
		Contributions:        res.Contributions,
		AccessTokenExpiresAt: exp.UTC().Format(time.RFC3339),
	})
}

// ensureAccessToken returns the expiry of a valid access token, minting a fresh
// fixed-24h one only when none is currently valid (R10 — the window is not
// rolling).
func (h *handlers) ensureAccessToken(w http.ResponseWriter, r *http.Request) time.Time {
	if c, err := r.Cookie(accessCookie); err == nil {
		if claims, err := token.Verify(h.cfg.TokenSecret, c.Value); err == nil {
			return time.Unix(claims.Exp, 0)
		}
	}
	tok, exp, err := token.Mint(h.cfg.TokenSecret, h.cfg.AccessTTL)
	if err != nil {
		h.logger.Error("mint access token", "err", err)
		return time.Time{}
	}
	setCookie(w, r, accessCookie, tok, h.cfg.AccessTTL)
	return exp
}

// --- shared helpers ----------------------------------------------------------

// localize returns display content for a subject in displayLang, lazily fetching
// and caching a missing translation, falling back to English (docs/06 §Step 4).
func (h *handlers) localize(ctx context.Context, subj model.Subject, displayLang string) model.Translation {
	if tr, found, err := h.store.Translation(ctx, subj.ID, displayLang); err == nil && found {
		return tr
	}
	if displayLang != "en" {
		if tr, ok := h.fetchTranslation(ctx, subj, displayLang); ok {
			return tr
		}
	}
	if en, found, _ := h.store.Translation(ctx, subj.ID, "en"); found {
		return en
	}
	// Last resort (should not happen — en is seeded): canonical name only.
	return model.Translation{
		SubjectID:    subj.ID,
		Lang:         "en",
		Name:         subj.CanonicalName,
		WikipediaURL: "https://en.wikipedia.org/wiki/" + strings.ReplaceAll(subj.CanonicalName, " ", "_"),
	}
}

// ensureExtract backfills a missing lead paragraph for a subject shown in a
// matchup. Rows seeded before extracts were stored (and lazily-cached languages)
// have a description but no extract; this best-effort refetch fills it in the
// language already resolved, so the card gets its inline paragraph and the row
// is upserted for next time. Only the matchup needs this (≤2 calls, decaying to
// zero as the pool's extracts fill); the leaderboard doesn't show extracts.
func (h *handlers) ensureExtract(ctx context.Context, subj model.Subject, tr model.Translation) model.Translation {
	if tr.Extract != "" || tr.Lang == "" {
		return tr
	}
	if fresh, ok := h.fetchTranslation(ctx, subj, tr.Lang); ok && fresh.Extract != "" {
		return fresh
	}
	return tr
}

// fetchTranslation resolves the title for displayLang, fetches the summary, and
// caches it. Best-effort: returns ok=false on any failure (caller falls back).
func (h *handlers) fetchTranslation(ctx context.Context, subj model.Subject, displayLang string) (model.Translation, bool) {
	title, err := h.ingest.SitelinkTitle(ctx, subj.WikidataID, displayLang)
	if err != nil || title == "" {
		return model.Translation{}, false
	}
	sum, err := h.ingest.Summary(ctx, displayLang, title)
	if err != nil {
		return model.Translation{}, false
	}
	url := sum.WikipediaURL
	if url == "" {
		url = "https://" + displayLang + ".wikipedia.org/wiki/" + strings.ReplaceAll(title, " ", "_")
	}
	name := sum.Name
	if name == "" {
		name = subj.CanonicalName
	}
	if err := h.store.UpsertTranslation(ctx, subj.ID, displayLang, name, sum.Description, sum.Extract, sum.ImageURL, url); err != nil {
		h.logger.Warn("cache translation", "qid", subj.WikidataID, "lang", displayLang, "err", err)
	}
	return model.Translation{
		SubjectID: subj.ID, Lang: displayLang, Name: name,
		Description: sum.Description, Extract: sum.Extract, ImageURL: sum.ImageURL, WikipediaURL: url,
	}, true
}

func publicOf(subj model.Subject, tr model.Translation) model.SubjectPublic {
	p := model.SubjectPublic{
		ID:           subj.ID,
		WikidataID:   subj.WikidataID,
		Name:         tr.Name,
		Description:  tr.Description,
		Extract:      tr.Extract,
		ImageURL:     tr.ImageURL,
		WikipediaURL: tr.WikipediaURL,
	}
	if subj.DiedAt != nil {
		p.Deceased = true
		p.DiedYear = subj.DiedAt.Year()
	}
	return p
}
