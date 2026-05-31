// Package http wires the chi router, middleware, and API handlers. Handlers are
// thin: parse the request, call a service, encode the response
// (docs/02-architecture.md §Backend internals).
package http

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/thdelmas/e-agora/backend/internal/config"
	"github.com/thdelmas/e-agora/backend/internal/store"
)

// NewRouter builds the top-level HTTP handler with cross-cutting middleware and
// the /api routes. Endpoints not yet implemented in this milestone return 501.
func NewRouter(cfg config.Config, db *store.Store, logger *slog.Logger) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	h := &handlers{cfg: cfg, store: db, logger: logger}

	r.Route("/api", func(r chi.Router) {
		r.Get("/healthz", h.healthz)

		// Wired in later milestones (docs/07-roadmap.md M3–M4):
		r.Get("/matchup", notImplemented)
		r.Get("/human/challenge", notImplemented)
		r.Post("/human/verify", notImplemented)
		r.Post("/votes", notImplemented)
		r.Get("/leaderboard", notImplemented)
		r.Post("/subjects", notImplemented)
		r.Get("/subjects/search", notImplemented)
		r.Get("/me", notImplemented)
	})

	return r
}

// handlers carries shared dependencies for the API handlers.
type handlers struct {
	cfg    config.Config
	store  *store.Store
	logger *slog.Logger
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "This endpoint is not implemented yet.")
}
