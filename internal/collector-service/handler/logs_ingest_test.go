package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opentrace/opentrace/pkg/schema"
)

// mockLogRepo captures inserted events and can simulate errors.
type mockLogRepo struct {
	insertErr error
	inserted  []schema.LogEvent
}

func (m *mockLogRepo) BulkInsert(_ context.Context, events []schema.LogEvent) error {
	if m.insertErr != nil {
		return m.insertErr
	}
	m.inserted = append(m.inserted, events...)
	return nil
}

func nopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestHandler(repo *mockLogRepo) http.HandlerFunc {
	return IngestLogs(repo, nopLogger(), 1000, 5*1024*1024)
}

// validEventJSON returns a JSON object for a single valid log event.
func validEventJSON(overrides map[string]any) map[string]any {
	e := map[string]any{
		"timestamp":    time.Now().UTC().Format(time.RFC3339Nano),
		"service_name": "test-service",
		"severity":     "info",
		"body":         "test message",
	}
	for k, v := range overrides {
		e[k] = v
	}
	return e
}

func postJSON(t *testing.T, handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func decodeProblem(t *testing.T, rr *httptest.ResponseRecorder) schema.Problem {
	t.Helper()
	var p schema.Problem
	if err := json.NewDecoder(rr.Body).Decode(&p); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	return p
}

func TestIngestLogs(t *testing.T) {
	tests := []struct {
		name           string
		buildBody      func() any
		wantStatus     int
		wantViolation  string // field path that must appear in violations (empty = skip check)
	}{
		{
			name: "valid batch of 3 events",
			buildBody: func() any {
				return map[string]any{"events": []any{
					validEventJSON(nil),
					validEventJSON(map[string]any{"severity": "warn", "body": "warn message"}),
					validEventJSON(map[string]any{"severity": "error", "body": "error message"}),
				}}
			},
			wantStatus: http.StatusAccepted,
		},
		{
			name:       "empty events array",
			buildBody:  func() any { return map[string]any{"events": []any{}} },
			wantStatus: http.StatusBadRequest,
			wantViolation: "events",
		},
		{
			name: "invalid log level",
			buildBody: func() any {
				return map[string]any{"events": []any{
					validEventJSON(map[string]any{"severity": "VERBOSE"}),
				}}
			},
			wantStatus:    http.StatusBadRequest,
			wantViolation: "events[0].severity",
		},
		{
			name: "missing service_name",
			buildBody: func() any {
				return map[string]any{"events": []any{
					validEventJSON(map[string]any{"service_name": ""}),
				}}
			},
			wantStatus:    http.StatusBadRequest,
			wantViolation: "events[0].service_name",
		},
		{
			name: "missing body",
			buildBody: func() any {
				return map[string]any{"events": []any{
					validEventJSON(map[string]any{"body": ""}),
				}}
			},
			wantStatus:    http.StatusBadRequest,
			wantViolation: "events[0].body",
		},
		{
			name: "body exceeds 32768 chars",
			buildBody: func() any {
				return map[string]any{"events": []any{
					validEventJSON(map[string]any{"body": strings.Repeat("x", 32769)}),
				}}
			},
			wantStatus:    http.StatusBadRequest,
			wantViolation: "events[0].body",
		},
		{
			name: "batch exceeds 1000 events",
			buildBody: func() any {
				events := make([]any, 1001)
				for i := range events {
					events[i] = validEventJSON(nil)
				}
				return map[string]any{"events": events}
			},
			wantStatus:    http.StatusBadRequest,
			wantViolation: "events",
		},
		{
			name: "too many log_attributes keys",
			buildBody: func() any {
				attrs := map[string]any{}
				for i := 0; i <= 100; i++ {
					attrs[fmt.Sprintf("key_%d", i)] = "value"
				}
				return map[string]any{"events": []any{
					validEventJSON(map[string]any{"log_attributes": attrs}),
				}}
			},
			wantStatus:    http.StatusBadRequest,
			wantViolation: "events[0].log_attributes",
		},
		{
			name: "valid with all optional fields",
			buildBody: func() any {
				return map[string]any{"events": []any{
					validEventJSON(map[string]any{
						"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
						"span_id":  "00f067aa0ba902b7",
						"log_attributes": map[string]any{
							"user_id": "u_123",
						},
					}),
				}}
			},
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockLogRepo{}
			handler := newTestHandler(repo)
			rr := postJSON(t, handler, tt.buildBody())

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rr.Code, tt.wantStatus, rr.Body.String())
			}

			if tt.wantViolation != "" {
				p := decodeProblem(t, rr)
				found := false
				for _, v := range p.Violations {
					if v.Field == tt.wantViolation {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected violation on field %q, got violations: %+v", tt.wantViolation, p.Violations)
				}
			}
		})
	}
}

func TestIngestLogs_NonJSONBody(t *testing.T) {
	repo := &mockLogRepo{}
	h := newTestHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestIngestLogs_PayloadTooLarge(t *testing.T) {
	repo := &mockLogRepo{}
	// Use a 1-byte limit to force the 413 without building a 5 MB body in tests.
	h := IngestLogs(repo, nopLogger(), 1000, 1)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logs",
		strings.NewReader(`{"events":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want 413", rr.Code)
	}
}

func TestIngestLogs_ReturnsAcceptedResponse(t *testing.T) {
	repo := &mockLogRepo{}
	h := newTestHandler(repo)
	rr := postJSON(t, h, map[string]any{"events": []any{validEventJSON(nil)}})

	if rr.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rr.Code)
	}

	var resp schema.IngestLogsResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Accepted != 1 {
		t.Errorf("accepted = %d, want 1", resp.Accepted)
	}
	if resp.BatchID == "" {
		t.Error("batch_id must not be empty")
	}
}

func TestIngestLogs_MultipleViolationsReturned(t *testing.T) {
	repo := &mockLogRepo{}
	h := newTestHandler(repo)
	// Both severity and service_name are invalid — expect both violations.
	rr := postJSON(t, h, map[string]any{"events": []any{
		validEventJSON(map[string]any{
			"severity":     "INVALID",
			"service_name": "",
		}),
	}})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
	p := decodeProblem(t, rr)
	if len(p.Violations) < 2 {
		t.Errorf("expected >=2 violations, got %d: %+v", len(p.Violations), p.Violations)
	}
}

func TestValidateIngestRequest_AllViolations(t *testing.T) {
	req := &schema.IngestLogsRequest{
		Events: []schema.LogEvent{
			{
				// timestamp is zero (empty)
				ServiceName: "",
				Severity:    "BAD",
				Body:        "",
			},
		},
	}
	violations := validateIngestRequest(req, 1000)
	fields := map[string]bool{}
	for _, v := range violations {
		fields[v.Field] = true
	}
	for _, want := range []string{"events[0].severity", "events[0].service_name", "events[0].body", "events[0].timestamp"} {
		if !fields[want] {
			t.Errorf("expected violation for field %q, got %v", want, violations)
		}
	}
}
