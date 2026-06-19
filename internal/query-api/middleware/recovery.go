package middleware

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/opentrace/opentrace/pkg/schema"
)

// Recovery catches panics and returns a structured 500 response.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						slog.String("request_id", GetRequestID(r.Context())),
						slog.String("panic", fmt.Sprintf("%v", rec)),
						slog.String("stack", string(debug.Stack())),
					)
					schema.WriteProblem(w, schema.Problem{
						Type:   "https://opentrace.io/errors/internal",
						Title:  "Internal Server Error",
						Status: http.StatusInternalServerError,
					})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
