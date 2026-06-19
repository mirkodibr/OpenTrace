package repository

import (
	"context"

	"github.com/opentrace/opentrace/internal/query-api/handler"
	"github.com/opentrace/opentrace/pkg/schema"
)

// LogReadRepository is the read contract for the query-api.
type LogReadRepository interface {
	QueryLogs(ctx context.Context, params *handler.LogsQueryParams) ([]schema.LogEvent, error)
}
