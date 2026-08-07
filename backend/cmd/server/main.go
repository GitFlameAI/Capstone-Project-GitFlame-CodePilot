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

	"gitflame-codepilot/backend/internal/config"
	"gitflame-codepilot/backend/internal/httpapi"
	"gitflame-codepilot/backend/internal/observability"
)

// shutdownTimeout bounds how long a redeploy waits for in-flight work.
//
// It is generous on purpose: a plan request that is waiting for repository
// indexing can legitimately take minutes, and killing it means the user's action
// is lost. Docker's own default is 10 seconds, so docker-compose.yml raises
// stop_grace_period to match this value.
const shutdownTimeout = 30 * time.Second

func main() {
	cfg := config.Load()
	logger := observability.Init("backend", cfg.LogLevel, cfg.LogFormat)

	server, err := httpapi.New(cfg)
	if err != nil {
		logger.Error("startup_failed", slog.String("event", "startup_failed"), slog.String("error", err.Error()))
		os.Exit(1)
	}

	// The root context is cancelled on SIGINT/SIGTERM. Background work started
	// from it — the metrics collector, repository indexing jobs — stops with it.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server.StartMetricsCollector(ctx)

	build := observability.Build("backend")
	logger.Info("server_started",
		slog.String("event", "server_started"),
		slog.String("addr", cfg.Addr),
		slog.String("version", build.Version),
		slog.String("commit", build.Commit),
		slog.String("dispatch_mode", cfg.DispatchMode),
	)

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           server.Router(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_stopped", slog.String("event", "server_stopped"), slog.String("error", err.Error()))
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutdown_started",
			slog.String("event", "shutdown_started"),
			slog.String("timeout", shutdownTimeout.String()))

		// Stop accepting new connections and let in-flight requests finish.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			logger.Warn("shutdown_incomplete",
				slog.String("event", "shutdown_incomplete"), slog.String("error", err.Error()))
		}

		// Background indexing jobs are given the same budget. A job that is still
		// running is not fatal — the next request re-verifies the index and
		// rebuilds it if needed — but finishing is cheaper than redoing.
		if remaining := server.WaitForBackgroundWork(shutdownCtx); remaining > 0 {
			logger.Warn("shutdown_background_work_abandoned",
				slog.String("event", "shutdown_background_work_abandoned"),
				slog.Int("indexing_jobs", remaining))
		}
		logger.Info("shutdown_complete", slog.String("event", "shutdown_complete"))
	}
}
