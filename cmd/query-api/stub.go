package main

import (
	"net/http"

	"github.com/opentrace/opentrace/internal/query-api/handler"
	"github.com/opentrace/opentrace/pkg/schema"
)

// stubRepository satisfies handler.LogReadRepository while the real PostgreSQL
// read layer is implemented in Phase 1 (Day 11).
type stubRepository struct{}

func (s *stubRepository) QueryLogs(_ *http.Request, _ *handler.LogsQueryParams) ([]schema.LogEvent, error) {
	return []schema.LogEvent{}, nil
}
