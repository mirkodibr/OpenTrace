package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/opentrace/opentrace/internal/collector-service/config"
	"github.com/opentrace/opentrace/internal/collector-service/handler"
	"github.com/opentrace/opentrace/internal/collector-service/middleware"
)

// Server wraps the HTTP server with its router and dependencies.
type Server struct {
	http   *http.Server
	logger *slog.Logger
}

// New constructs and returns a fully configured Server.
// The caller owns the lifecycle: call ListenAndServe to start,
// and Shutdown to drain in-flight requests.
func New(cfg *config.Config, repo handler.LogRepository, logger *slog.Logger) *Server {
	r := chi.NewRouter()

	// Middleware chain — order is significant. Decompress runs before the
	// ingest handler so its http.MaxBytesReader bounds DECOMPRESSED bytes
	// (gzip-bomb defence, see middleware.Decompress).
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestID)
	r.Use(middleware.Logging(logger))
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.Decompress)

	// Routes
	r.Get("/healthz", handler.Health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/logs", handler.IngestLogs(repo, logger, cfg.MaxBatchSize, cfg.MaxPayloadBytes))
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

// ListenAndServe starts the HTTP server. It blocks until the server is shut down.
func (s *Server) ListenAndServe() error {
	s.logger.Info("collector-service listening", slog.String("addr", s.http.Addr))
	return s.http.ListenAndServe()
}

// Shutdown gracefully drains in-flight requests within the provided context deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("collector-service shutting down")
	return s.http.Shutdown(ctx)
}
