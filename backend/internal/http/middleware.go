package http

import (
	"context"
	"net/http"
	"time"

	"github.com/thdelmas/e-agora/backend/internal/model"
	"github.com/thdelmas/e-agora/backend/internal/token"
)

const (
	sessionCookie = "eagora_session"   // anonymous contribution counter (not auth)
	accessCookie  = "eagora_lb_access" // 24h leaderboard access token (R10)
	sessionMaxAge = 365 * 24 * time.Hour
)

type ctxKey int

const sessionKey ctxKey = iota

// sessionFrom returns the session attached by sessionMW (zero value if absent).
func sessionFrom(ctx context.Context) model.Session {
	s, _ := ctx.Value(sessionKey).(model.Session)
	return s
}

// sessionMW reads the anonymous session cookie (minting one if absent), refreshes
// it in the store, and attaches the session to the request context. This is NOT
// authentication (R3) — it identifies a browser to count contributions and carry
// the human-verified status.
func (h *handlers) sessionMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := ""
		if c, err := r.Cookie(sessionCookie); err == nil {
			id = c.Value
		}
		if id == "" {
			newID, err := token.NewID()
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal", "Could not start a session.")
				return
			}
			id = newID
			setCookie(w, r, sessionCookie, id, sessionMaxAge)
		}
		sess, err := h.store.TouchSession(r.Context(), id)
		if err != nil {
			h.logger.Error("session touch", "err", err)
			writeError(w, http.StatusInternalServerError, "internal", "Session error.")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}

// setCookie writes an httpOnly, SameSite=Lax cookie; Secure when served over TLS.
func setCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
}
