package main

import (
	"net/http"

	"github.com/opentrace/opentrace/pkg/schema"
)

// stubRepository satisfies handler.LogRepository while the real PostgreSQL
// persistence layer is implemented in Phase 1 (Day 10).
type stubRepository struct{}

func (s *stubRepository) BulkInsert(_ *http.Request, events []schema.LogEvent) error {
	_ = events
	return nil
}
