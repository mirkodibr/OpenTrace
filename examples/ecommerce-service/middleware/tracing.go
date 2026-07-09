// Package middleware provides the OpenTrace SDK integration for the example
// e-commerce service: one middleware that gives every request a request ID
// and a request-scoped child logger carried through the context.
package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	opentrace "github.com/opentrace/opentrace-go"
)

type contextKey string

const loggerKey contextKey = "opentrace_logger"

// statusRecorder captures the response status for the request summary log.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Tracing derives a child logger per request with request-scoped fields
// pre-set, stores it in the context for handlers, and emits a summary event
// on completion. All handler log lines automatically carry request_id
// without manual field passing.
func Tracing(base *opentrace.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = uuid.NewString()
			}
			w.Header().Set("X-Request-ID", requestID)

			reqLogger := base.With(
				opentrace.String("request_id", requestID),
				opentrace.String("http.method", r.Method),
				opentrace.String("http.path", r.URL.Path),
				opentrace.String("http.user_agent", r.UserAgent()),
			)
			ctx := context.WithValue(r.Context(), loggerKey, reqLogger)

			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			start := time.Now()
			next.ServeHTTP(rec, r.WithContext(ctx))
			elapsed := time.Since(start)

			if rec.status >= 500 {
				reqLogger.Error("request failed",
					opentrace.Int("http.status_code", rec.status),
					opentrace.Duration("duration", elapsed),
				)
				return
			}
			reqLogger.Info("request completed",
				opentrace.Int("http.status_code", rec.status),
				opentrace.Duration("duration", elapsed),
			)
		})
	}
}

// LoggerFromContext returns the request-scoped logger stored by Tracing.
// Falls back to a nop logger so handlers never nil-check.
func LoggerFromContext(ctx context.Context) *opentrace.Logger {
	if l, ok := ctx.Value(loggerKey).(*opentrace.Logger); ok {
		return l
	}
	return opentrace.NewNop()
}
