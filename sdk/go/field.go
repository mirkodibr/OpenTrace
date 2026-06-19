package opentrace

import "time"

// fieldType enumerates the concrete type stored in a Field. Using an enum
// rather than an interface avoids heap allocation for scalar types.
type fieldType uint8

const (
	typeString  fieldType = iota
	typeInt64
	typeFloat64
	typeBool
	typeDuration
	typeError
	typeAny
)

// Field is a typed key-value pair for structured log attributes.
// It is a value type (no pointers for scalar types) to keep the hot path
// allocation-free. Callers must not mutate a Field after passing it to a
// logger method.
type Field struct {
	Key        string
	ftype      fieldType
	StringVal  string
	Int64Val   int64
	Float64Val float64
	BoolVal    bool
	// Interface is only populated for typeAny and typeError; it causes one
	// heap allocation which is documented in the SDK allocation budget.
	Interface interface{}
}

// String constructs a string Field. Zero allocations.
func String(key, value string) Field {
	return Field{Key: key, ftype: typeString, StringVal: value}
}

// Int constructs an int Field. Zero allocations.
func Int(key string, value int) Field {
	return Field{Key: key, ftype: typeInt64, Int64Val: int64(value)}
}

// Int64 constructs an int64 Field. Zero allocations.
func Int64(key string, value int64) Field {
	return Field{Key: key, ftype: typeInt64, Int64Val: value}
}

// Float64 constructs a float64 Field. Zero allocations.
func Float64(key string, value float64) Field {
	return Field{Key: key, ftype: typeFloat64, Float64Val: value}
}

// Bool constructs a bool Field. Zero allocations.
func Bool(key string, value bool) Field {
	return Field{Key: key, ftype: typeBool, BoolVal: value}
}

// Duration constructs a time.Duration Field stored as nanoseconds. Zero allocations.
func Duration(key string, value time.Duration) Field {
	return Field{Key: key, ftype: typeDuration, Int64Val: int64(value)}
}

// Err constructs an error Field with key "error". One allocation (interface boxing).
func Err(err error) Field {
	if err == nil {
		return Field{Key: "error", ftype: typeString, StringVal: "<nil>"}
	}
	return Field{Key: "error", ftype: typeError, StringVal: err.Error(), Interface: err}
}

// Any constructs a Field for an arbitrary value using fmt.Sprintf-style boxing.
// This is the escape hatch — prefer typed constructors to avoid allocations.
func Any(key string, value interface{}) Field {
	return Field{Key: key, ftype: typeAny, Interface: value}
}
