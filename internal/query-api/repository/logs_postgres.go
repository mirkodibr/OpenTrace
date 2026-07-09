package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/opentrace/opentrace/internal/query-api/handler"
	"github.com/opentrace/opentrace/pkg/schema"
)

// selectCols lists every column returned by QueryLogs — must stay in sync
// with the rows.Scan call below.
const selectCols = `
	id, timestamp, received_at, service_name, severity, severity_text,
	body, trace_id, span_id, resource_attributes, log_attributes, schema_url`

// PostgresLogReadRepository implements LogReadRepository using pgxpool.
type PostgresLogReadRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresLogReadRepository constructs a read repository.
func NewPostgresLogReadRepository(pool *pgxpool.Pool) *PostgresLogReadRepository {
	return &PostgresLogReadRepository{pool: pool}
}

// QueryLogs executes a dynamic multi-filter query and returns up to
// params.Limit log events in ascending timestamp order.
func (r *PostgresLogReadRepository) QueryLogs(ctx context.Context, params *handler.LogsQueryParams) ([]schema.LogEvent, error) {
	query, args := buildLogsQuery(params)

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query logs: %w", err)
	}
	defer rows.Close()

	var events []schema.LogEvent
	for rows.Next() {
		var (
			e                schema.LogEvent
			id               int64
			receivedAt       time.Time
			severityText     *string
			traceID          *string
			spanID           *string
			resourceAttrsRaw []byte
			logAttrsRaw      []byte
			schemaURL        *string
		)

		if err := rows.Scan(
			&id,
			&e.Timestamp,
			&receivedAt,
			&e.ServiceName,
			&e.Severity,
			&severityText,
			&e.Body,
			&traceID,
			&spanID,
			&resourceAttrsRaw,
			&logAttrsRaw,
			&schemaURL,
		); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		e.ID = fmt.Sprintf("%d", id)
		e.ReceivedAt = receivedAt
		if severityText != nil {
			e.SeverityText = *severityText
		}
		if traceID != nil {
			e.TraceID = *traceID
		}
		if spanID != nil {
			e.SpanID = *spanID
		}
		if schemaURL != nil {
			e.SchemaURL = *schemaURL
		}
		if len(resourceAttrsRaw) > 0 {
			_ = json.Unmarshal(resourceAttrsRaw, &e.ResourceAttributes)
		}
		if len(logAttrsRaw) > 0 {
			_ = json.Unmarshal(logAttrsRaw, &e.LogAttributes)
		}

		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}
	return events, nil
}

// buildLogsQuery constructs the parameterised SQL for the query-api.
// User-supplied values are always bound as parameters ($n) — never interpolated
// into the query string — so SQL injection is structurally impossible.
func buildLogsQuery(p *handler.LogsQueryParams) (string, []interface{}) {
	var (
		sb   strings.Builder
		args []interface{}
		n    int
	)

	ph := func(v interface{}) string {
		n++
		args = append(args, v)
		return fmt.Sprintf("$%d", n)
	}

	sb.WriteString("SELECT ")
	sb.WriteString(selectCols)
	sb.WriteString(" FROM logs WHERE timestamp >= ")
	sb.WriteString(ph(p.StartTime))
	sb.WriteString(" AND timestamp <= ")
	sb.WriteString(ph(p.EndTime))

	if p.ServiceName != nil {
		sb.WriteString(" AND service_name = ")
		sb.WriteString(ph(*p.ServiceName))
	}
	if p.Level != nil {
		sb.WriteString(" AND severity = ")
		sb.WriteString(ph(*p.Level))
		sb.WriteString("::log_severity")
	}
	if p.Keyword != nil {
		sb.WriteString(" AND body_tsv @@ plainto_tsquery('english', ")
		sb.WriteString(ph(*p.Keyword))
		sb.WriteString(")")
	}
	if p.Cursor != nil {
		// Keyset pagination: skip rows at or before the cursor position.
		// The (timestamp, id) composite comparison is correct even when
		// multiple events share the same timestamp — id breaks the tie.
		sb.WriteString(" AND (timestamp, id) > (")
		sb.WriteString(ph(p.Cursor.Timestamp))
		sb.WriteString(", ")
		sb.WriteString(ph(p.Cursor.ID))
		sb.WriteString(")")
	}

	sb.WriteString(" ORDER BY timestamp ASC, id ASC LIMIT ")
	sb.WriteString(ph(p.Limit))

	return sb.String(), args
}
