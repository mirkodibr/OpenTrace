package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/opentrace/opentrace/internal/collector-service/config"
	"github.com/opentrace/opentrace/internal/collector-service/handler"
	"github.com/opentrace/opentrace/internal/collector-service/repository"
	"github.com/opentrace/opentrace/internal/collector-service/server"
	"github.com/opentrace/opentrace/internal/database"
)

// version is injected at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	logger.Info("starting collector-service", slog.String("version", version))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	cfg.Log(logger)

	ctx := context.Background()

	// Wire up the real repository or fall back to the no-op stub when no
	// database URL is configured (useful for local integration tests).
	var repo handler.LogRepository
	var repoCloser interface{ Close() }

	if cfg.DatabaseURL != "" {
		pool, err := database.NewPool(ctx, cfg.DatabaseURL, logger)
		if err != nil {
			logger.Error("database connection failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		pgRepo := repository.NewPostgresLogRepository(pool, logger)
		repo = pgRepo
		repoCloser = pgRepo
		defer pool.Close()
	} else {
		logger.Warn("COLLECTOR_DATABASE_URL not set — using no-op stub repository")
		stub := &stubRepository{}
		repo = stub
		repoCloser = stub
	}

	srv := server.New(cfg, repo, logger)

	serverErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("shutdown signal received", slog.String("signal", sig.String()))
	case err := <-serverErr:
		logger.Error("server error", slog.String("error", err.Error()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	repoCloser.Close()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("collector-service stopped cleanly")
}
