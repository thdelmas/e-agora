// Command server is the e-agora backend entrypoint: it loads config, wires the
// chi router, and serves HTTP with graceful shutdown.
//
// M0 scaffold: the router is up and /api/healthz responds. PostgreSQL wiring,
// ingestion, voting, and the access-token gate land in later milestones
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
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           eagorahttp.NewRouter(cfg, logger),
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
