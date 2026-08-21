// Package wire holds the SDK's shared value types (Field, LogEvent, Level)
// and the event pool. It exists so the pipeline packages (internal/batcher,
// internal/exporter) can share these types with the public opentrace package
// without an import cycle (ADR-005 D3). The root package re-exports
// everything here via type aliases and thin wrappers.
package wire

import "time"

// FieldType enumerates the concrete type stored in a Field. Using an enum
// rather than an interface avoids heap allocation for scalar types.
type FieldType uint8

const (
	TypeString FieldType = iota
	TypeInt64
	TypeFloat64
	TypeBool
	TypeDuration
	TypeError
	TypeAny
	TypeObject      // nested object: Interface holds []Field
	TypeStringSlice // Interface holds []string
	TypeMap         // Interface holds map[string]string
)

// Field is a typed key-value pair for structured log attributes.
// It is a value type (no pointers for scalar types) to keep the hot path
// allocation-free. Callers must not mutate a Field after passing it to a
// logger method.
type Field struct {
	Key        string
	Type       FieldType
	StringVal  string
	Int64Val   int64
	Float64Val float64
	BoolVal    bool
	// Interface is only populated for TypeAny and TypeError; it causes one
	// heap allocation which is documented in the SDK allocation budget.
	Interface interface{}
}

// String constructs a string Field. Zero allocations.
func String(key, value string) Field {
	return Field{Key: key, Type: TypeString, StringVal: value}
}

// Int constructs an int Field. Zero allocations.
func Int(key string, value int) Field {
	return Field{Key: key, Type: TypeInt64, Int64Val: int64(value)}
}

// Int64 constructs an int64 Field. Zero allocations.
func Int64(key string, value int64) Field {
	return Field{Key: key, Type: TypeInt64, Int64Val: value}
}

// Float64 constructs a float64 Field. Zero allocations.
func Float64(key string, value float64) Field {
	return Field{Key: key, Type: TypeFloat64, Float64Val: value}
}

// Bool constructs a bool Field. Zero allocations.
func Bool(key string, value bool) Field {
	return Field{Key: key, Type: TypeBool, BoolVal: value}
}

// Duration constructs a time.Duration Field stored as nanoseconds. Zero allocations.
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Type: TypeDuration, Int64Val: int64(value)}
}

// Err constructs an error Field with key "error". One allocation (interface boxing).
func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", Type: TypeString, StringVal: "<nil>"}
	}
	return Field{Key: "error", Type: TypeError, StringVal: err.Error(), Interface: err}
}

// Any constructs a Field for an arbitrary value. One allocation (interface
// boxing). This is the escape hatch — prefer typed constructors.
func Any(key string, value interface{}) Field {
	return Field{Key: key, Type: TypeAny, Interface: value}
}

// Object constructs a nested-object Field from child fields. Nesting may be
// arbitrary in the SDK; the collector flattens anything deeper than 5
// levels to dot-notation keys (see collector sanitisation). Budget: 1-2
// allocs/op (the child slice boxing) — nested structure cannot be expressed
// without it.
func Object(key string, fields ...Field) Field {
	return Field{Key: key, Type: TypeObject, Interface: fields}
}

// StringSlice constructs a Field holding an array of strings. 1 alloc
// (slice header boxing). The caller must not mutate values afterwards.
func StringSlice(key string, values []string) Field {
	return Field{Key: key, Type: TypeStringSlice, Interface: values}
}

// Map constructs a Field from a string map, serialised as a one-level
// nested object. 1 alloc (map header boxing). The caller must not mutate
// m afterwards.
func Map(key string, m map[string]string) Field {
	return Field{Key: key, Type: TypeMap, Interface: m}
}
