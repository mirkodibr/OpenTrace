package exporter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// serialize renders a batch into the collector wire format (ADR-005 D4,
// field mapping in ADR-006) and writes it to buf:
//
//	{"events":[{ "timestamp": <RFC3339Nano>, "service_name": ...,
//	             "severity": <lowercase level>, "body": <message>,
//	             "trace_id"?, "span_id"?,
//	             "resource_attributes": {OTel keys}, "log_attributes": {...} }]}
//
// Fields are flattened into log_attributes with last-wins key collision
// semantics; trace_id/span_id fields are lifted to top level. Durations are
// serialised as float64 milliseconds (matching the duration_ms convention
// used across the platform).
//
// This is the correctness-first baseline using encoding/json; it runs only
// on the exporter goroutine, never on the hot path. Day 32 replaces the
// inner loop with a hand-written writer if benchmarks justify it.
func serialize(buf *bytes.Buffer, batch []*wire.LogEvent, res wire.Resource) error {
	resourceAttrs := map[string]interface{}{
		"service.name":           res.ServiceName,
		"service.version":        res.ServiceVersion,
		"deployment.environment": res.Environment,
		"host.name":              res.Hostname,
		"process.pid":            res.PID,
	}

	events := make([]map[string]interface{}, 0, len(batch))
	for _, e := range batch {
		evt := map[string]interface{}{
			"timestamp":           e.Timestamp.UTC().Format(time.RFC3339Nano),
			"service_name":        res.ServiceName,
			"severity":            e.Level.String(),
			"body":                e.Message,
			"resource_attributes": resourceAttrs,
		}
		if len(e.Fields) > 0 {
			attrs := make(map[string]interface{}, len(e.Fields))
			for i := range e.Fields {
				f := &e.Fields[i]
				val := fieldValue(f)
				switch f.Key {
				case "trace_id":
					evt["trace_id"] = val
				case "span_id":
					evt["span_id"] = val
				default:
					attrs[f.Key] = val // last-wins on collision (ADR-006)
				}
			}
			if len(attrs) > 0 {
				evt["log_attributes"] = attrs
			}
		}
		events = append(events, evt)
	}

	enc := json.NewEncoder(buf)
	return enc.Encode(map[string]interface{}{"events": events})
}

// fieldValue converts a Field to its JSON representation.
func fieldValue(f *wire.Field) interface{} {
	switch f.Type {
	case wire.TypeString, wire.TypeError:
		return f.StringVal
	case wire.TypeInt64:
		return f.Int64Val
	case wire.TypeFloat64:
		return f.Float64Val
	case wire.TypeBool:
		return f.BoolVal
	case wire.TypeDuration:
		// Milliseconds with sub-ms precision, e.g. 1500000ns -> 1.5
		return float64(f.Int64Val) / float64(time.Millisecond)
	case wire.TypeObject:
		children, _ := f.Interface.([]wire.Field)
		obj := make(map[string]interface{}, len(children))
		for i := range children {
			obj[children[i].Key] = fieldValue(&children[i]) // recursive
		}
		return obj
	case wire.TypeStringSlice:
		values, _ := f.Interface.([]string)
		return values
	case wire.TypeMap:
		m, _ := f.Interface.(map[string]string)
		return m
	case wire.TypeAny:
		return anyValue(f.Interface)
	default:
		return f.StringVal
	}
}

// anyValue guards the Any escape hatch: values encoding/json cannot marshal
// (channels, funcs, cyclic structures) must not poison the whole batch, so
// they degrade to their fmt representation.
func anyValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch v.(type) {
	case string, bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, float32, float64,
		json.Marshaler:
		return v
	}
	if _, err := json.Marshal(v); err != nil {
		return fmt.Sprint(v)
	}
	return v
}
