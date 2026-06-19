package main

import (
	"context"

	"github.com/opentrace/opentrace/internal/query-api/handler"
	"github.com/opentrace/opentrace/pkg/schema"
)

// stubRepository satisfies handler.LogReadRepository when no database is configured.
type stubRepository struct{}

func (s *stubRepository) QueryLogs(_ context.Context, _ *handler.LogsQueryParams) ([]schema.LogEvent, error) {
	return []schema.LogEvent{}, nil
}
