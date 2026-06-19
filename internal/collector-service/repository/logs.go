package repository

import (
	"context"
	"errors"

	"github.com/opentrace/opentrace/pkg/schema"
)

// Sentinel errors for callers to inspect with errors.Is.
var (
	ErrConstraintViolation = errors.New("constraint violation")
	ErrRetryable           = errors.New("retryable database error")
	ErrResourceExhausted   = errors.New("database resource exhausted")
	ErrCancelled           = errors.New("operation cancelled by database")
	ErrInternal            = errors.New("internal database error")
)

// LogWriteRepository is the write contract for the collector-service.
// BulkInsert must be safe for concurrent callers and must treat all events
// in a single call as an atomic unit (all-or-nothing via transaction).
type LogWriteRepository interface {
	BulkInsert(ctx context.Context, events []schema.LogEvent) error
	Close()
}
