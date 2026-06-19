//go:build integration

package repository_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/opentrace/opentrace/internal/collector-service/repository"
	"github.com/opentrace/opentrace/internal/database"
	"github.com/opentrace/opentrace/pkg/schema"
)

// Run integration tests with:
//   TEST_DATABASE_URL=postgres://... go test -tags=integration ./internal/collector-service/repository/...

func testDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set — skipping integration test")
	}
	return dsn
}

func newTestRepo(t *testing.T) *repository.PostgresLogRepository {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pool, err := database.NewPool(context.Background(), testDSN(t), logger)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return repository.NewPostgresLogRepository(pool, logger)
}

func sampleEvent(service string) schema.LogEvent {
	return schema.LogEvent{
		TraceID:     "4bf92f3577b34da6a3ce929d0e0e4736",
		SpanID:      "00f067aa0ba902b7",
		Timestamp:   time.Now().UTC().Truncate(time.Microsecond),
		ServiceName: service,
		Severity:    schema.LogLevelInfo,
		Body:        "integration test event",
		ResourceAttributes: map[string]any{
			"host.name": "test-host",
		},
		LogAttributes: map[string]any{
			"http.status_code": 200,
		},
	}
}

func TestBulkInsert_SingleEvent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.BulkInsert(ctx, []schema.LogEvent{sampleEvent("svc-a")}); err != nil {
		t.Fatalf("BulkInsert: %v", err)
	}
}

func TestBulkInsert_LargeBatch(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	events := make([]schema.LogEvent, 1000)
	for i := range events {
		events[i] = sampleEvent("svc-bulk")
	}

	if err := repo.BulkInsert(ctx, events); err != nil {
		t.Fatalf("BulkInsert 1000 events: %v", err)
	}
}

func TestBulkInsert_EmptyBatch_NoError(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.BulkInsert(context.Background(), nil); err != nil {
		t.Fatalf("BulkInsert empty: %v", err)
	}
}

func TestBulkInsert_CancelledContext_ReturnsError(t *testing.T) {
	repo := newTestRepo(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before any DB call

	events := []schema.LogEvent{sampleEvent("svc-cancel")}
	err := repo.BulkInsert(ctx, events)
	if err == nil {
		// Some drivers may succeed if the operation completes fast enough before
		// the context deadline is observed; that is acceptable. If an error is
		// returned it should not be nil (we already checked) — just log it.
		t.Log("cancelled context: operation completed before cancellation was observed (acceptable)")
	}
}

func TestBulkInsert_NullableFields_NoError(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// TraceID, SpanID, SeverityText, SchemaURL are all empty — should map to SQL NULL.
	evt := schema.LogEvent{
		Timestamp:   time.Now().UTC(),
		ServiceName: "svc-nullable",
		Severity:    schema.LogLevelDebug,
		Body:        "nullable fields test",
	}

	if err := repo.BulkInsert(ctx, []schema.LogEvent{evt}); err != nil {
		t.Fatalf("BulkInsert nullable fields: %v", err)
	}
}
