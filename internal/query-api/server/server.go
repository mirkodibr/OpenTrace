package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/opentrace/opentrace/internal/query-api/config"
	"github.com/opentrace/opentrace/internal/query-api/handler"
	"github.com/opentrace/opentrace/internal/query-api/middleware"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	http   *http.Server
	logger *slog.Logger
}

// New constructs a fully configured query-api HTTP server.
func New(cfg *config.Config, repo handler.LogReadRepository, logger *slog.Logger) *Server {
	r := chi.NewRouter()

	r.Use(chimw.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(logger))
	r.Use(middleware.Recovery(logger))

	r.Get("/healthz", handler.Health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/logs", handler.QueryLogs(repo, logger, cfg.DefaultPageSize, cfg.MaxTimeRangeDays))
	})

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           r,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ReadHeaderTimeout: 2 * time.Second,
	}

	return &Server{http: httpServer, logger: logger}
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	s.logger.Info("query-api listening", slog.String("addr", s.http.Addr))
	return s.http.ListenAndServe()
}

// Shutdown gracefully drains in-flight requests.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("query-api shutting down")
	return s.http.Shutdown(ctx)
}
