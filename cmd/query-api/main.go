package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/opentrace/opentrace/internal/database"
	"github.com/opentrace/opentrace/internal/query-api/config"
	"github.com/opentrace/opentrace/internal/query-api/handler"
	queryrepo "github.com/opentrace/opentrace/internal/query-api/repository"
	"github.com/opentrace/opentrace/internal/query-api/server"
)

var version = "dev"

func main() {
	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	logger.Info("starting query-api", slog.String("version", version))

	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration error", slog.String("error", err.Error()))
		os.Exit(1)
	}
	cfg.Log(logger)

	ctx := context.Background()

	var repo handler.LogReadRepository

	if cfg.DatabaseURL != "" {
		pool, err := database.NewPool(ctx, cfg.DatabaseURL, logger)
		if err != nil {
			logger.Error("database connection failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		defer pool.Close()
		repo = queryrepo.NewPostgresLogReadRepository(pool)
	} else {
		logger.Warn("QUERY_API_DATABASE_URL not set — using stub repository")
		repo = &stubRepository{}
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

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	logger.Info("query-api stopped cleanly")
}
