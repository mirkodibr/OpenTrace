package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/opentrace/opentrace/internal/collector-service/middleware"
	"github.com/opentrace/opentrace/pkg/schema"
)

// LogRepository is the persistence contract for log ingest.
type LogRepository interface {
	BulkInsert(ctx context.Context, events []schema.LogEvent) error
}

// IngestLogs handles POST /api/v1/logs.
// It validates the incoming batch, enqueues it for persistence, and returns
// 202 Accepted. All error responses are RFC 7807 Problem Details.
func IngestLogs(repo LogRepository, logger *slog.Logger, maxBatchSize int, maxPayloadBytes int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := middleware.GetRequestID(r.Context())
		reqLogger := logger.With(slog.String("request_id", reqID))

		// Guard against oversized payloads before attempting any decode.
		r.Body = http.MaxBytesReader(w, r.Body, maxPayloadBytes)

		var req schema.IngestLogsRequest
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil {
			if err.Error() == "http: request body too large" {
				schema.WriteProblem(w, schema.Problem{
					Type:   "https://opentrace.io/errors/payload-too-large",
					Title:  "Payload Too Large",
					Status: http.StatusRequestEntityTooLarge,
					Detail: "Request body exceeds the maximum allowed size.",
				})
				return
			}
			schema.WriteProblem(w, schema.Problem{
				Type:   "https://opentrace.io/errors/invalid-json",
				Title:  "Bad Request",
				Status: http.StatusBadRequest,
				Detail: "Request body must be valid JSON.",
			})
			return
		}

		if violations := validateIngestRequest(&req, maxBatchSize); len(violations) > 0 {
			schema.WriteProblem(w, schema.Problem{
				Type:       "https://opentrace.io/errors/validation-failed",
				Title:      "Validation Failed",
				Status:     http.StatusBadRequest,
				Detail:     "One or more fields failed validation.",
				Violations: violations,
			})
			return
		}

		if err := repo.BulkInsert(r.Context(), req.Events); err != nil {
			reqLogger.Error("bulk insert failed", slog.String("error", err.Error()))
			schema.WriteProblem(w, schema.Problem{
				Type:   "https://opentrace.io/errors/internal",
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
			})
			return
		}

		reqLogger.Info("logs ingested",
			slog.Int("batch_size", len(req.Events)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(schema.IngestLogsResponse{
			BatchID:               uuid.NewString(),
			Accepted:              len(req.Events),
			EstimatedProcessingMs: 500,
		})
	}
}

// validateIngestRequest performs full validation and returns all violations.
func validateIngestRequest(req *schema.IngestLogsRequest, maxBatchSize int) []schema.ValidationError {
	var violations []schema.ValidationError

	if len(req.Events) == 0 {
		violations = append(violations, schema.ValidationError{
			Field:   "events",
			Message: "must contain at least one event",
		})
		return violations
	}
	if len(req.Events) > maxBatchSize {
		violations = append(violations, schema.ValidationError{
			Field:   "events",
			Message: fmt.Sprintf("batch size %d exceeds maximum of %d", len(req.Events), maxBatchSize),
		})
	}

	for i, e := range req.Events {
		field := func(f string) string { return fmt.Sprintf("events[%d].%s", i, f) }

		if e.Timestamp.IsZero() {
			violations = append(violations, schema.ValidationError{
				Field:   field("timestamp"),
				Message: "required; must be a valid RFC3339 timestamp",
			})
		}
		if !e.Severity.Validate() {
			violations = append(violations, schema.ValidationError{
				Field:   field("severity"),
				Message: "must be one of: debug, info, warn, error, fatal",
			})
		}
		if e.Body == "" {
			violations = append(violations, schema.ValidationError{
				Field:   field("body"),
				Message: "required; must be non-empty",
			})
		} else if len(e.Body) > 32768 {
			violations = append(violations, schema.ValidationError{
				Field:   field("body"),
				Message: "must not exceed 32,768 characters",
			})
		}
		if e.ServiceName == "" {
			violations = append(violations, schema.ValidationError{
				Field:   field("service_name"),
				Message: "required; must be non-empty",
			})
		} else if len(e.ServiceName) > 255 {
			violations = append(violations, schema.ValidationError{
				Field:   field("service_name"),
				Message: "must not exceed 255 characters",
			})
		}
		if len(e.LogAttributes) > 100 {
			violations = append(violations, schema.ValidationError{
				Field:   field("log_attributes"),
				Message: "must not contain more than 100 keys",
			})
		}
	}

	return violations
}
