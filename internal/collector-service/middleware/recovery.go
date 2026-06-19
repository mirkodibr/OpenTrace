package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/opentrace/opentrace/pkg/schema"
)

// Recovery catches panics, logs the stack trace with the request ID, and
// returns an RFC 7807 500 response so the caller receives a structured error
// instead of a raw connection reset.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					reqID := GetRequestID(r.Context())
					logger.Error("panic recovered",
						slog.String("request_id", reqID),
						slog.String("panic", fmt.Sprintf("%v", rec)),
						slog.String("stack", string(debug.Stack())),
					)
					schema.WriteProblem(w, schema.Problem{
						Type:   "https://opentrace.io/errors/internal",
						Title:  "Internal Server Error",
						Status: http.StatusInternalServerError,
						Detail: "An unexpected error occurred. The request ID has been logged.",
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
