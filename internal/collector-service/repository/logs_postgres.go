package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentrace/opentrace/pkg/schema"
)

// logColumns matches the INSERT column list for the logs table.
// Excludes id (GENERATED ALWAYS AS IDENTITY) and body_tsv (GENERATED ALWAYS).
var logColumns = []string{
	"trace_id", "span_id", "timestamp", "received_at",
	"service_name", "severity", "severity_text", "body",
	"resource_attributes", "log_attributes", "schema_url",
}

// PostgresLogRepository implements LogWriteRepository using pgx CopyFrom for
// high-throughput bulk inserts.
type PostgresLogRepository struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	wg     sync.WaitGroup // tracks in-flight BulkInsert calls for graceful shutdown
}

// NewPostgresLogRepository constructs a repository backed by pool.
func NewPostgresLogRepository(pool *pgxpool.Pool, logger *slog.Logger) *PostgresLogRepository {
	return &PostgresLogRepository{pool: pool, logger: logger}
}

// BulkInsert writes all events to PostgreSQL in a single CopyFrom call wrapped
// in a transaction. Either all rows are committed or none are.
func (r *PostgresLogRepository) BulkInsert(ctx context.Context, events []schema.LogEvent) error {
	if len(events) == 0 {
		return nil
	}

	r.wg.Add(1)
	defer r.wg.Done()

	start := time.Now()

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return classifyPgError(fmt.Errorf("begin transaction: %w", err))
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	src := &logCopySource{events: events}
	n, err := tx.CopyFrom(ctx, pgx.Identifier{"logs"}, logColumns, src)
	if err != nil {
		return classifyPgError(fmt.Errorf("copy from: %w", err))
	}

	if err = tx.Commit(ctx); err != nil {
		return classifyPgError(fmt.Errorf("commit: %w", err))
	}

	r.logger.Debug("bulk insert complete",
		slog.Int64("rows", n),
		slog.Int64("duration_ms", time.Since(start).Milliseconds()),
	)
	return nil
}

// Close waits for all in-flight BulkInsert calls to complete, then closes
// the pool. Callers should invoke Close during graceful shutdown before
// calling pool.Close() independently.
func (r *PostgresLogRepository) Close() {
	r.wg.Wait()
	r.logger.Info("log repository closed cleanly")
}

// logCopySource implements pgx.CopyFromSource without allocating an
// intermediate [][]interface{} slice; it streams rows directly from events.
type logCopySource struct {
	events []schema.LogEvent
	idx    int
	err    error
}

func (s *logCopySource) Next() bool {
	s.idx++
	return s.idx <= len(s.events)
}

func (s *logCopySource) Values() ([]interface{}, error) {
	e := s.events[s.idx-1]
	now := time.Now().UTC()

	resourceAttrs, err := marshalJSONB(e.ResourceAttributes)
	if err != nil {
		return nil, fmt.Errorf("resource_attributes: %w", err)
	}
	logAttrs, err := marshalJSONB(e.LogAttributes)
	if err != nil {
		return nil, fmt.Errorf("log_attributes: %w", err)
	}

	return []interface{}{
		nilIfEmpty(e.TraceID),
		nilIfEmpty(e.SpanID),
		e.Timestamp.UTC(),
		now,
		e.ServiceName,
		string(e.Severity),
		nilIfEmpty(e.SeverityText),
		e.Body,
		resourceAttrs,
		logAttrs,
		nilIfEmpty(e.SchemaURL),
	}, nil
}

func (s *logCopySource) Err() error { return s.err }

// classifyPgError maps PostgreSQL error codes to typed sentinel errors so
// callers can distinguish retryable from permanent failures.
func classifyPgError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("%w: %w", ErrInternal, err)
	}
	switch pgErr.Code[:2] {
	case "23": // integrity constraint violation
		return fmt.Errorf("%w: %s", ErrConstraintViolation, pgErr.Message)
	case "40": // serialization failure / deadlock
		return fmt.Errorf("%w: %s", ErrRetryable, pgErr.Message)
	case "53": // insufficient resources
		return fmt.Errorf("%w: %s", ErrResourceExhausted, pgErr.Message)
	case "57": // operator intervention / cancel
		return fmt.Errorf("%w: %s", ErrCancelled, pgErr.Message)
	default:
		return fmt.Errorf("%w: %s (sqlstate=%s)", ErrInternal, pgErr.Message, pgErr.Code)
	}
}

// marshalJSONB encodes m to JSON bytes suitable for the pgx JSONB type.
// Returns nil when m is nil or empty so the column stores NULL.
func marshalJSONB(m map[string]any) ([]byte, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return json.Marshal(m)
}

// nilIfEmpty returns nil when s is the empty string, otherwise returns &s.
// This preserves the SQL NULL vs empty string distinction.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
