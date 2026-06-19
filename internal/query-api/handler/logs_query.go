package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/opentrace/opentrace/internal/query-api/middleware"
	"github.com/opentrace/opentrace/pkg/schema"
)

// LogsQueryParams holds validated, parsed query parameters for GET /api/v1/logs.
type LogsQueryParams struct {
	ServiceName *string
	Level       *string
	StartTime   time.Time
	EndTime     time.Time
	Keyword     *string
	Cursor      *LogsCursor
	Limit       int
}

// LogsCursor represents the keyset pagination position: the last seen
// (timestamp, id) pair from the previous page.
type LogsCursor struct {
	Timestamp time.Time `json:"ts"`
	ID        int64     `json:"id"`
}

// EncodeCursor serialises c to an opaque, URL-safe base64 string.
func EncodeCursor(c LogsCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses an opaque cursor string back into a LogsCursor.
func DecodeCursor(s string) (*LogsCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor encoding: %w", err)
	}
	var c LogsCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("invalid cursor structure: %w", err)
	}
	return &c, nil
}

// LogReadRepository is the read contract for the query-api.
type LogReadRepository interface {
	QueryLogs(r *http.Request, params *LogsQueryParams) ([]schema.LogEvent, error)
}

// logsQueryResponse is the JSON envelope returned by GET /api/v1/logs.
type logsQueryResponse struct {
	Data        []schema.LogEvent `json:"data"`
	NextCursor  *string           `json:"next_cursor"`
	QueryTimeMs int64             `json:"query_time_ms"`
}

// QueryLogs handles GET /api/v1/logs.
func QueryLogs(repo LogReadRepository, logger *slog.Logger, defaultPageSize, maxTimeRangeDays int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := middleware.GetRequestID(r.Context())
		reqLogger := logger.With(slog.String("request_id", reqID))

		params, err := parseLogsQueryParams(r, defaultPageSize, maxTimeRangeDays)
		if err != nil {
			schema.WriteProblem(w, schema.Problem{
				Type:   "https://opentrace.io/errors/invalid-query",
				Title:  "Bad Request",
				Status: http.StatusBadRequest,
				Detail: err.Error(),
			})
			return
		}

		events, err := repo.QueryLogs(r, params)
		if err != nil {
			reqLogger.Error("query failed", slog.String("error", err.Error()))
			schema.WriteProblem(w, schema.Problem{
				Type:   "https://opentrace.io/errors/internal",
				Title:  "Internal Server Error",
				Status: http.StatusInternalServerError,
			})
			return
		}

		var nextCursor *string
		if len(events) == params.Limit {
			last := events[len(events)-1]
			c := EncodeCursor(LogsCursor{Timestamp: last.Timestamp, ID: mustParseID(last.ID)})
			nextCursor = &c
		}

		queryTimeMs := time.Since(start).Milliseconds()
		reqLogger.Info("logs queried",
			slog.Int("result_count", len(events)),
			slog.Int64("query_time_ms", queryTimeMs),
		)

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Query-Time-Ms", strconv.FormatInt(queryTimeMs, 10))
		_ = json.NewEncoder(w).Encode(logsQueryResponse{
			Data:        events,
			NextCursor:  nextCursor,
			QueryTimeMs: queryTimeMs,
		})
	}
}

func parseLogsQueryParams(r *http.Request, defaultLimit, maxTimeRangeDays int) (*LogsQueryParams, error) {
	q := r.URL.Query()
	params := &LogsQueryParams{Limit: defaultLimit}

	startStr := q.Get("start_time")
	endStr := q.Get("end_time")
	if startStr == "" || endStr == "" {
		return nil, fmt.Errorf("start_time and end_time are required")
	}

	var err error
	params.StartTime, err = time.Parse(time.RFC3339, startStr)
	if err != nil {
		return nil, fmt.Errorf("start_time: must be RFC3339 format")
	}
	params.EndTime, err = time.Parse(time.RFC3339, endStr)
	if err != nil {
		return nil, fmt.Errorf("end_time: must be RFC3339 format")
	}
	if params.EndTime.Before(params.StartTime) {
		return nil, fmt.Errorf("end_time must be after start_time")
	}
	if params.EndTime.Sub(params.StartTime) > time.Duration(maxTimeRangeDays)*24*time.Hour {
		return nil, fmt.Errorf("time range must not exceed %d days", maxTimeRangeDays)
	}

	if v := q.Get("service_name"); v != "" {
		params.ServiceName = &v
	}
	if v := q.Get("level"); v != "" {
		level := schema.LogLevel(v)
		if !level.Validate() {
			return nil, fmt.Errorf("level: must be one of debug, info, warn, error, fatal")
		}
		params.Level = &v
	}
	if v := q.Get("keyword"); v != "" {
		if len(v) > 256 {
			return nil, fmt.Errorf("keyword: must not exceed 256 characters")
		}
		params.Keyword = &v
	}
	if v := q.Get("cursor"); v != "" {
		c, err := DecodeCursor(v)
		if err != nil {
			return nil, fmt.Errorf("cursor: %w", err)
		}
		params.Cursor = c
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			return nil, fmt.Errorf("limit: must be an integer between 1 and 1000")
		}
		params.Limit = n
	}

	return params, nil
}

// mustParseID extracts an int64 ID from a LogEvent.ID string.
// Returns 0 for non-numeric IDs (handled by cursor on the DB side).
func mustParseID(id string) int64 {
	n, _ := strconv.ParseInt(id, 10, 64)
	return n
}
