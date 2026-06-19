package main

import (
	"context"

	"github.com/opentrace/opentrace/pkg/schema"
)

// stubRepository satisfies handler.LogRepository while the real PostgreSQL
// persistence layer is not yet wired (used when COLLECTOR_DATABASE_URL is absent).
type stubRepository struct{}

func (s *stubRepository) BulkInsert(_ context.Context, events []schema.LogEvent) error {
	_ = events
	return nil
}

func (s *stubRepository) Close() {}
