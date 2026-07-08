//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	writerepo "github.com/opentrace/opentrace/internal/collector-service/repository"
	"github.com/opentrace/opentrace/internal/database"
	queryhandler "github.com/opentrace/opentrace/internal/query-api/handler"
	queryrepo "github.com/opentrace/opentrace/internal/query-api/repository"
	"github.com/opentrace/opentrace/pkg/schema"
)

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

func setupRepos(t *testing.T) (*writerepo.PostgresLogRepository, *queryrepo.PostgresLogReadRepository) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pool, err := database.NewPool(context.Background(), testDSN(t), logger)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return writerepo.NewPostgresLogRepository(pool, logger),
		queryrepo.NewPostgresLogReadRepository(pool)
}

func TestQueryLogs_RoundTrip(t *testing.T) {
	writeRepo, readRepo := setupRepos(t)
	ctx := context.Background()

	// Write a distinct event that we can query back.
	ts := time.Now().UTC().Truncate(time.Microsecond)
	evt := schema.LogEvent{
		Timestamp:   ts,
		ServiceName: "integration-test-svc",
		Severity:    schema.LogLevelError,
		Body:        "unique round-trip probe",
	}
	if err := writeRepo.BulkInsert(ctx, []schema.LogEvent{evt}); err != nil {
		t.Fatalf("write: %v", err)
	}

	params := &queryhandler.LogsQueryParams{
		StartTime:   ts.Add(-time.Second),
		EndTime:     ts.Add(time.Second),
		ServiceName: strPtr("integration-test-svc"),
		Level:       strPtr("error"),
		Limit:       10,
	}
	events, err := readRepo.QueryLogs(ctx, params)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event, got none")
	}

	found := false
	for _, e := range events {
		if e.Body == "unique round-trip probe" && e.ServiceName == "integration-test-svc" {
			found = true
			break
		}
	}
	if !found {
		t.Error("inserted event not found in query results")
	}
}

func TestQueryLogs_KeysetPagination(t *testing.T) {
	writeRepo, readRepo := setupRepos(t)
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Millisecond)
	events := make([]schema.LogEvent, 5)
	for i := range events {
		events[i] = schema.LogEvent{
			Timestamp:   base.Add(time.Duration(i) * time.Millisecond),
			ServiceName: "pagination-test-svc",
			Severity:    schema.LogLevelInfo,
			Body:        "page event",
		}
	}
	if err := writeRepo.BulkInsert(ctx, events); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Page 1: limit=2
	p1, err := readRepo.QueryLogs(ctx, &queryhandler.LogsQueryParams{
		StartTime:   base.Add(-time.Second),
		EndTime:     base.Add(time.Second),
		ServiceName: strPtr("pagination-test-svc"),
		Limit:       2,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(p1) != 2 {
		t.Fatalf("page 1: expected 2 events, got %d", len(p1))
	}

	// Build cursor from last event on page 1.
	last := p1[len(p1)-1]
	cursor := &queryhandler.LogsCursor{Timestamp: last.Timestamp, ID: mustParseID(last.ID)}

	// Page 2: should not re-include the events from page 1.
	p2, err := readRepo.QueryLogs(ctx, &queryhandler.LogsQueryParams{
		StartTime:   base.Add(-time.Second),
		EndTime:     base.Add(time.Second),
		ServiceName: strPtr("pagination-test-svc"),
		Cursor:      cursor,
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	for _, e := range p2 {
		if e.ID == last.ID {
			t.Errorf("page 2 contains duplicate event ID %s (cursor anchor should be excluded)", e.ID)
		}
	}
}

func TestQueryLogs_FullTextSearch(t *testing.T) {
	writeRepo, readRepo := setupRepos(t)
	ctx := context.Background()

	ts := time.Now().UTC().Truncate(time.Microsecond)
	if err := writeRepo.BulkInsert(ctx, []schema.LogEvent{
		{Timestamp: ts, ServiceName: "fts-svc", Severity: schema.LogLevelInfo, Body: "xylophone cascade failure"},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}

	events, err := readRepo.QueryLogs(ctx, &queryhandler.LogsQueryParams{
		StartTime: ts.Add(-time.Second),
		EndTime:   ts.Add(time.Second),
		Keyword:   strPtr("xylophone"),
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	found := false
	for _, e := range events {
		if e.Body == "xylophone cascade failure" {
			found = true
			break
		}
	}
	if !found {
		t.Error("full-text search did not return the expected event")
	}
}

// helpers

func strPtr(s string) *string { return &s }

func mustParseID(s string) int64 {
	var n int64
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n
}
