package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/thdelmas/e-agora/backend/internal/human"
)

// humanChallenge issues an anonymous dissent-based humanity challenge (R12).
func (h *handlers) humanChallenge(w http.ResponseWriter, r *http.Request) {
	if h.human == nil {
		writeError(w, http.StatusInternalServerError, "internal",
			"Humanity check unavailable.")
		return
	}
	ch, err := h.human.NewChallenge()
	if err != nil {
		h.logger.Error("human: new challenge", "err", err)
		writeError(w, http.StatusInternalServerError, "internal",
			"Could not issue a challenge.")
		return
	}
	writeJSON(w, http.StatusOK, ch)
}

type humanVerifyRequest struct {
	ChallengeID string       `json:"challengeId"`
	Answer      string       `json:"answer"`
	Timing      human.Timing `json:"timing"`
}

type humanVerifyResponse struct {
	Verified           bool           `json:"verified"`
	HumanVerifiedUntil string         `json:"humanVerifiedUntil,omitempty"`
	Reason             string         `json:"reason,omitempty"`
	ChallengeID        string         `json:"challengeId,omitempty"`
	Prompt             string         `json:"prompt,omitempty"`
	Kind               string         `json:"kind,omitempty"`
	Options            []human.Option `json:"options,omitempty"`
}

// humanVerify checks an answer; on success it sets the session's human-verified
// window (R12), on failure it returns a fresh challenge.
func (h *handlers) humanVerify(w http.ResponseWriter, r *http.Request) {
	if h.human == nil {
		writeError(w, http.StatusInternalServerError, "internal",
			"Humanity check unavailable.")
		return
	}
	sess := sessionFrom(r.Context())

	var req humanVerifyRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"Malformed request body.")
		return
	}

	ok, reason := h.human.Verify(req.ChallengeID, req.Answer, req.Timing)
	if ok {
		until := time.Now().Add(h.cfg.HumanTTL)
		if err := h.store.MarkHuman(r.Context(), sess.ID, until); err != nil {
			h.logger.Error("human: mark verified", "err", err)
			writeError(w, http.StatusInternalServerError, "internal",
				"Could not record verification.")
			return
		}
		writeJSON(w, http.StatusOK, humanVerifyResponse{
			Verified:           true,
			HumanVerifiedUntil: until.UTC().Format(time.RFC3339),
		})
		return
	}

	// Failed — hand back a fresh challenge so the client can retry.
	ch, err := h.human.NewChallenge()
	if err != nil {
		h.logger.Error("human: new challenge", "err", err)
		writeError(w, http.StatusInternalServerError, "internal",
			"Could not issue a challenge.")
		return
	}
	writeJSON(w, http.StatusOK, humanVerifyResponse{
		Verified:    false,
		Reason:      reason,
		ChallengeID: ch.ChallengeID,
		Prompt:      ch.Prompt,
		Kind:        ch.Kind,
		Options:     ch.Options,
	})
}
