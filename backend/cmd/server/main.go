// Command server is the e-agora backend entrypoint: it loads config, connects to
// PostgreSQL, applies migrations, wires the chi router, and serves HTTP with
// graceful shutdown.
//
// As of M1 the database is wired and /api/healthz reports the live subject
// count. Ingestion, voting, and the access-token gate land in later milestones
// (see docs/07-roadmap.md).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thdelmas/e-agora/backend/internal/config"
	eagorahttp "github.com/thdelmas/e-agora/backend/internal/http"
	"github.com/thdelmas/e-agora/backend/internal/store"
	"github.com/thdelmas/e-agora/backend/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()

	// Connect to PostgreSQL and apply migrations before serving. A short,
	// bounded startup context keeps a dead database from hanging boot.
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelStartup()

	db, err := store.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("cannot connect to PostgreSQL",
			"err", err,
			"hint", "is the database running? try: docker compose up -d db")
		os.Exit(1)
	}
	defer db.Close()

	applied, err := db.Migrate(startupCtx, migrations.FS)
	if err != nil {
		logger.Error("migrations failed", "err", err)
		os.Exit(1)
	}
	logger.Info("migrations up to date", "applied_this_boot", applied)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           eagorahttp.NewRouter(cfg, db, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Run the server until an interrupt signal arrives.
	go func() {
		logger.Info("e-agora server starting", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	logger.Info("stopped")
}
