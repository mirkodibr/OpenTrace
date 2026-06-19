package repository

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/opentrace/opentrace/internal/query-api/handler"
)

func ptr[T any](v T) *T { return &v }

func TestBuildLogsQuery_BaseCase(t *testing.T) {
	start := time.Now().Add(-time.Hour)
	end := time.Now()
	params := &handler.LogsQueryParams{
		StartTime: start,
		EndTime:   end,
		Limit:     100,
	}

	q, args := buildLogsQuery(params)

	if !strings.Contains(q, "timestamp >=") {
		t.Errorf("query missing start_time filter: %s", q)
	}
	if !strings.Contains(q, "timestamp <=") {
		t.Errorf("query missing end_time filter: %s", q)
	}
	if !strings.Contains(q, "LIMIT") {
		t.Errorf("query missing LIMIT: %s", q)
	}
	if len(args) != 3 { // start, end, limit
		t.Errorf("expected 3 args, got %d: %v", len(args), args)
	}
}

func TestBuildLogsQuery_AllFilters(t *testing.T) {
	params := &handler.LogsQueryParams{
		StartTime:   time.Now().Add(-time.Hour),
		EndTime:     time.Now(),
		ServiceName: ptr("payment-service"),
		Level:       ptr("error"),
		Keyword:     ptr("timeout"),
		Cursor:      &handler.LogsCursor{Timestamp: time.Now().Add(-30 * time.Minute), ID: 42},
		Limit:       50,
	}

	q, args := buildLogsQuery(params)

	checks := map[string]string{
		"service_name filter": "service_name =",
		"severity filter":     "severity =",
		"severity cast":       "::log_severity",
		"full-text filter":    "body_tsv @@",
		"cursor filter":       "(timestamp, id) >",
		"ordering":            "ORDER BY timestamp ASC, id ASC",
	}
	for name, fragment := range checks {
		if !strings.Contains(q, fragment) {
			t.Errorf("%s: fragment %q not found in query:\n%s", name, fragment, q)
		}
	}

	// start, end, service, severity, keyword, cursor.ts, cursor.id, limit = 8 args
	if len(args) != 8 {
		t.Errorf("expected 8 args, got %d", len(args))
	}
}

func TestBuildLogsQuery_NoSQLInjection(t *testing.T) {
	malicious := "'; DROP TABLE logs; --"
	params := &handler.LogsQueryParams{
		StartTime:   time.Now().Add(-time.Hour),
		EndTime:     time.Now(),
		ServiceName: &malicious,
		Limit:       10,
	}

	q, args := buildLogsQuery(params)

	// The malicious string must appear in args, not in the query string.
	for _, arg := range args {
		if s, ok := arg.(string); ok && s == malicious {
			return // found in args — correct
		}
	}
	if strings.Contains(q, malicious) {
		t.Errorf("malicious string was interpolated into the query: %s", q)
	}
}

func TestBuildLogsQuery_ParameterNumbering(t *testing.T) {
	params := &handler.LogsQueryParams{
		StartTime:   time.Now().Add(-time.Hour),
		EndTime:     time.Now(),
		ServiceName: ptr("svc"),
		Limit:       10,
	}

	q, args := buildLogsQuery(params)

	// Verify $1..$n are all present. Use fmt.Sprintf so n >= 10 is handled correctly.
	for i := 1; i <= len(args); i++ {
		placeholder := fmt.Sprintf("$%d", i)
		if !strings.Contains(q, placeholder) {
			t.Errorf("placeholder %s not found in query: %s", placeholder, q)
		}
	}
}
