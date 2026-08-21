package opentrace

import (
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// Field is a typed key-value pair for structured log attributes. It is a
// value type to keep the hot path allocation-free. Callers must not mutate
// a Field after passing it to a logger method.
//
// The concrete definition lives in internal/wire (ADR-005 D3) so the
// pipeline packages can share it; this alias keeps the public API here.
type Field = wire.Field

// String constructs a string Field. Zero allocations.
func String(key, value string) Field { return wire.String(key, value) }

// Int constructs an int Field. Zero allocations.
func Int(key string, value int) Field { return wire.Int(key, value) }

// Int64 constructs an int64 Field. Zero allocations.
func Int64(key string, value int64) Field { return wire.Int64(key, value) }

// Float64 constructs a float64 Field. Zero allocations.
func Float64(key string, value float64) Field { return wire.Float64(key, value) }

// Bool constructs a bool Field. Zero allocations.
func Bool(key string, value bool) Field { return wire.Bool(key, value) }

// Duration constructs a time.Duration Field stored as nanoseconds. Zero allocations.
func Duration(key string, value time.Duration) Field { return wire.Duration(key, value) }

// Err constructs an error Field with key "error". One allocation (interface boxing).
func Err(err error) Field { return wire.Err(err) }

// Any constructs a Field for an arbitrary value. One allocation (interface
// boxing). This is the escape hatch — prefer typed constructors.
func Any(key string, value interface{}) Field { return wire.Any(key, value) }

// Object constructs a nested-object Field from child fields, e.g.:
//
//	logger.Info("user action",
//	    opentrace.Object("user",
//	        opentrace.String("id", userID),
//	        opentrace.Object("permissions", opentrace.Bool("billing", true)),
//	    ),
//	)
//
// The collector flattens nesting deeper than 5 levels to dot-notation keys.
func Object(key string, fields ...Field) Field { return wire.Object(key, fields...) }

// StringSlice constructs a Field holding an array of strings.
func StringSlice(key string, values []string) Field { return wire.StringSlice(key, values) }

// Map constructs a Field from a string map (one-level nested object).
func Map(key string, m map[string]string) Field { return wire.Map(key, m) }
