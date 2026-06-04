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
	"github.com/thdelmas/e-agora/backend/internal/human"
	"github.com/thdelmas/e-agora/backend/internal/ingest"
	"github.com/thdelmas/e-agora/backend/internal/ratelimit"
	"github.com/thdelmas/e-agora/backend/internal/store"
	"github.com/thdelmas/e-agora/backend/internal/subjects"
)

// challengeTTL bounds how long a humanity challenge may be solved.
const challengeTTL = 10 * time.Minute

// NewRouter builds the top-level HTTP handler with cross-cutting middleware and
// the /api routes. Endpoints not yet implemented return 501.
func NewRouter(
	cfg config.Config, db *store.Store, logger *slog.Logger,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	client := ingest.NewClient()
	h := &handlers{
		cfg:     cfg,
		store:   db,
		logger:  logger,
		limiter: ratelimit.New(cfg.VoteBurst, cfg.VoteRate),
		ingest:  client,
		addsvc:  subjects.New(client, db),
	}
	if hc, err := human.New(cfg.TokenSecret, challengeTTL); err != nil {
		logger.Error("humanity check init failed", "err", err)
	} else {
		h.human = hc
	}

	r.Route("/api", func(r chi.Router) {
		r.Get("/healthz", h.healthz)
		// public transparency dashboard (ungated, no session)
		r.Get("/stats", h.stats)
		// public reference data for the pool picker (ungated)
		r.Get("/countries", h.countries)

		// Routes needing an anonymous session.
		r.Group(func(r chi.Router) {
			r.Use(h.sessionMW)
			r.Get("/matchup", h.matchup)
			r.Get("/human/challenge", h.humanChallenge)
			r.Post("/human/verify", h.humanVerify)
			r.Post("/votes", h.vote)
			r.Get("/subjects/recall", h.recall)
			r.Post("/proposals", h.proposal)
			r.Get("/me", h.me)
			r.Get("/leaderboard", h.leaderboard)
			r.Post("/subjects", h.addSubject)
			r.Get("/subjects/search", h.subjectsSearch)
			r.Get("/subjects/{id}", h.subject)
		})
	})

	// Production same-origin serving (M6): serve the built SPA for non-/api
	// paths when EAGORA_STATIC_DIR is set. In dev the Vite server does this.
	if cfg.StaticDir != "" {
		logger.Info("serving SPA", "dir", cfg.StaticDir)
		r.Handle("/*", spaHandler(cfg.StaticDir, cfg.PublicURL))
	}

	return r
}

// handlers carries shared dependencies for the API handlers.
type handlers struct {
	cfg     config.Config
	store   *store.Store
	logger  *slog.Logger
	limiter *ratelimit.Limiter
	human   *human.Checker
	ingest  *ingest.Client
	addsvc  *subjects.Service
}
