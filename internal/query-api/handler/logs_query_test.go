package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opentrace/opentrace/internal/query-api/handler"
	"github.com/opentrace/opentrace/pkg/schema"
	"log/slog"
	"os"
)

// stubRepo is a configurable in-memory LogReadRepository for handler tests.
type stubRepo struct {
	events []schema.LogEvent
	err    error
}

func (s *stubRepo) QueryLogs(_ context.Context, _ *handler.LogsQueryParams) ([]schema.LogEvent, error) {
	return s.events, s.err
}

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func mustGet(t *testing.T, url string, h http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestQueryLogs_MissingTimeRange(t *testing.T) {
	h := handler.QueryLogs(&stubRepo{}, newTestLogger(), 100, 7)

	w := mustGet(t, "/api/v1/logs", h)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	var prob schema.Problem
	if err := json.NewDecoder(w.Body).Decode(&prob); err != nil {
		t.Fatalf("response is not a Problem: %v", err)
	}
	if prob.Status != http.StatusBadRequest {
		t.Errorf("problem.status: want 400, got %d", prob.Status)
	}
}

func TestQueryLogs_EndBeforeStart(t *testing.T) {
	h := handler.QueryLogs(&stubRepo{}, newTestLogger(), 100, 7)

	url := "/api/v1/logs?start_time=2024-01-02T00:00:00Z&end_time=2024-01-01T00:00:00Z"
	w := mustGet(t, url, h)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestQueryLogs_TimeRangeExceedsMax(t *testing.T) {
	h := handler.QueryLogs(&stubRepo{}, newTestLogger(), 100, 7)

	// 8 days — exceeds maxTimeRangeDays=7
	url := "/api/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-09T00:00:00Z"
	w := mustGet(t, url, h)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestQueryLogs_InvalidLevel(t *testing.T) {
	h := handler.QueryLogs(&stubRepo{}, newTestLogger(), 100, 7)

	url := "/api/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-01T01:00:00Z&level=INVALID"
	w := mustGet(t, url, h)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestQueryLogs_InvalidCursor(t *testing.T) {
	h := handler.QueryLogs(&stubRepo{}, newTestLogger(), 100, 7)

	url := "/api/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-01T01:00:00Z&cursor=!!!notbase64"
	w := mustGet(t, url, h)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestQueryLogs_LimitOutOfRange(t *testing.T) {
	h := handler.QueryLogs(&stubRepo{}, newTestLogger(), 100, 7)

	url := "/api/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-01T01:00:00Z&limit=9999"
	w := mustGet(t, url, h)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestQueryLogs_RepositoryError_Returns500(t *testing.T) {
	repo := &stubRepo{err: errors.New("database unavailable")}
	h := handler.QueryLogs(repo, newTestLogger(), 100, 7)

	url := "/api/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-01T01:00:00Z"
	w := mustGet(t, url, h)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	var prob schema.Problem
	if err := json.NewDecoder(w.Body).Decode(&prob); err != nil {
		t.Fatalf("response is not a Problem: %v", err)
	}
	if prob.Status != http.StatusInternalServerError {
		t.Errorf("problem.status: want 500, got %d", prob.Status)
	}
}

func TestQueryLogs_EmptyResult(t *testing.T) {
	h := handler.QueryLogs(&stubRepo{events: []schema.LogEvent{}}, newTestLogger(), 100, 7)

	url := "/api/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-01T01:00:00Z"
	w := mustGet(t, url, h)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data       []schema.LogEvent `json:"data"`
		NextCursor *string           `json:"next_cursor"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty data, got %d events", len(resp.Data))
	}
	if resp.NextCursor != nil {
		t.Errorf("expected nil next_cursor for empty result, got %q", *resp.NextCursor)
	}
}

func TestQueryLogs_NextCursorPresentWhenResultEqualLimit(t *testing.T) {
	events := make([]schema.LogEvent, 2)
	for i := range events {
		events[i] = schema.LogEvent{
			ID:          "1",
			Timestamp:   time.Now(),
			ServiceName: "svc",
			Severity:    schema.LogLevelInfo,
			Body:        "msg",
		}
	}

	// limit=2, repo returns exactly 2 events → next_cursor should be set
	h := handler.QueryLogs(&stubRepo{events: events}, newTestLogger(), 2, 7)

	url := "/api/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-01T01:00:00Z&limit=2"
	w := mustGet(t, url, h)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data       []schema.LogEvent `json:"data"`
		NextCursor *string           `json:"next_cursor"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.NextCursor == nil {
		t.Error("expected next_cursor to be set when result count == limit")
	}
}

func TestQueryLogs_ResponseHeaders(t *testing.T) {
	h := handler.QueryLogs(&stubRepo{events: []schema.LogEvent{}}, newTestLogger(), 100, 7)

	url := "/api/v1/logs?start_time=2024-01-01T00:00:00Z&end_time=2024-01-01T01:00:00Z"
	w := mustGet(t, url, h)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type: want application/json, got %q", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control: want no-store, got %q", cc)
	}
	if xqt := w.Header().Get("X-Query-Time-Ms"); xqt == "" {
		t.Error("X-Query-Time-Ms header must be present")
	}
}
