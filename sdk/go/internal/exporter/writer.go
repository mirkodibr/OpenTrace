package exporter

import (
	"bytes"
	"encoding/json"
	"strconv"
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// serialize renders a batch into the collector wire format (ADR-005 D4,
// field mapping in ADR-006) by writing JSON directly to buf — no
// map[string]interface{} intermediate, no reflection. It runs only on the
// exporter goroutine (never the hot path); the point of hand-writing it is
// to cut per-batch allocations, not to satisfy the SDK's 0-alloc hot-path
// budget, which does not apply here.
//
// Semantically equivalent to serializeJSON (see serialize_equivalence_test.go):
// both produce output that decodes to the same structure. They are NOT
// byte-identical — two differences are intentional:
//
//  1. Key order: this writer preserves Fields slice order; serializeJSON
//     sorts map keys (encoding/json's map-marshalling behaviour). JSON
//     object key order carries no semantic meaning (RFC 8259 §4).
//  2. HTML escaping: encoding/json escapes <, >, & by default (for
//     embedding JSON in HTML). This writer does not — the payload is
//     consumed by a JSON API and stored in PostgreSQL JSONB, never
//     embedded in HTML, so the escaping is pure overhead here.
//
// Duplicate log_attributes keys (e.g. a child logger re-setting a parent
// field) are NOT deduplicated while writing — Go's json.Unmarshal resolves
// duplicate object keys to last-value-wins when the collector decodes the
// payload, reproducing the same last-wins semantic the map-building
// baseline enforced explicitly, without paying for a map here.
func serialize(buf *bytes.Buffer, batch []*wire.LogEvent, res wire.Resource) error {
	buf.WriteString(`{"events":[`)
	for i, e := range batch {
		if i > 0 {
			buf.WriteByte(',')
		}
		writeEvent(buf, e, res)
	}
	buf.WriteString(`]}`)
	return nil
}

func writeEvent(buf *bytes.Buffer, e *wire.LogEvent, res wire.Resource) {
	buf.WriteByte('{')
	buf.WriteString(`"timestamp":`)
	writeJSONString(buf, e.Timestamp.UTC().Format(time.RFC3339Nano))
	buf.WriteString(`,"service_name":`)
	writeJSONString(buf, res.ServiceName)
	buf.WriteString(`,"severity":`)
	writeJSONString(buf, e.Level.String())
	buf.WriteString(`,"body":`)
	writeJSONString(buf, e.Message)
	buf.WriteString(`,"resource_attributes":{"service.name":`)
	writeJSONString(buf, res.ServiceName)
	buf.WriteString(`,"service.version":`)
	writeJSONString(buf, res.ServiceVersion)
	buf.WriteString(`,"deployment.environment":`)
	writeJSONString(buf, res.Environment)
	buf.WriteString(`,"host.name":`)
	writeJSONString(buf, res.Hostname)
	buf.WriteString(`,"process.pid":`)
	writeInt64(buf, int64(res.PID))
	buf.WriteByte('}')

	hasAttrs := false
	for i := range e.Fields {
		f := &e.Fields[i]
		switch f.Key {
		case "trace_id":
			buf.WriteString(`,"trace_id":`)
			writeFieldValue(buf, f)
		case "span_id":
			buf.WriteString(`,"span_id":`)
			writeFieldValue(buf, f)
		default:
			hasAttrs = true
		}
	}

	if hasAttrs {
		buf.WriteString(`,"log_attributes":{`)
		first := true
		for i := range e.Fields {
			f := &e.Fields[i]
			if f.Key == "trace_id" || f.Key == "span_id" {
				continue
			}
			if !first {
				buf.WriteByte(',')
			}
			first = false
			writeJSONString(buf, f.Key)
			buf.WriteByte(':')
			writeFieldValue(buf, f)
		}
		buf.WriteByte('}')
	}
	buf.WriteByte('}')
}

// writeFieldValue writes f's value (never its key). Recurses for TypeObject.
func writeFieldValue(buf *bytes.Buffer, f *wire.Field) {
	switch f.Type {
	case wire.TypeString, wire.TypeError:
		writeJSONString(buf, f.StringVal)
	case wire.TypeInt64:
		writeInt64(buf, f.Int64Val)
	case wire.TypeFloat64:
		writeFloat64(buf, f.Float64Val)
	case wire.TypeBool:
		if f.BoolVal {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case wire.TypeDuration:
		// Milliseconds with sub-ms precision, e.g. 1500000ns -> 1.5
		writeFloat64(buf, float64(f.Int64Val)/float64(time.Millisecond))
	case wire.TypeObject:
		children, _ := f.Interface.([]wire.Field)
		buf.WriteByte('{')
		for i := range children {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, children[i].Key)
			buf.WriteByte(':')
			writeFieldValue(buf, &children[i]) // recursive
		}
		buf.WriteByte('}')
	case wire.TypeStringSlice:
		values, _ := f.Interface.([]string)
		buf.WriteByte('[')
		for i, v := range values {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, v)
		}
		buf.WriteByte(']')
	case wire.TypeMap:
		m, _ := f.Interface.(map[string]string)
		buf.WriteByte('{')
		first := true
		for k, v := range m {
			if !first {
				buf.WriteByte(',')
			}
			first = false
			writeJSONString(buf, k)
			buf.WriteByte(':')
			writeJSONString(buf, v)
		}
		buf.WriteByte('}')
	case wire.TypeAny:
		// The escape hatch: arbitrary Go values cannot be hand-written
		// without reflection, so this rare path falls back to
		// encoding/json. Correctness matters more than allocations here —
		// Any is documented as the non-zero-alloc constructor.
		b, err := json.Marshal(anyValue(f.Interface))
		if err != nil {
			writeJSONString(buf, "<unmarshalable>")
			return
		}
		buf.Write(b)
	default:
		writeJSONString(buf, f.StringVal)
	}
}

// writeInt64 appends n's decimal representation without heap allocation:
// scratch is stack-allocated and strconv.AppendInt never grows it (20 bytes
// covers every int64), so it does not escape.
func writeInt64(buf *bytes.Buffer, n int64) {
	var scratch [20]byte
	buf.Write(strconv.AppendInt(scratch[:0], n, 10))
}

// writeFloat64 appends f using the shortest round-trippable decimal
// representation (matches encoding/json's float formatting choice).
func writeFloat64(buf *bytes.Buffer, f float64) {
	var scratch [32]byte
	buf.Write(strconv.AppendFloat(scratch[:0], f, 'g', -1, 64))
}

const hexDigits = "0123456789abcdef"

// writeJSONString writes s as a quoted, escaped JSON string. Control
// characters (<0x20), the quote, and the backslash are escaped; valid UTF-8
// bytes >= 0x80 pass through unescaped (see the package doc for the
// intentional HTML-escaping divergence from encoding/json).
func writeJSONString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c != '"' && c != '\\' {
			continue
		}
		if start < i {
			buf.WriteString(s[start:i])
		}
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			buf.WriteString(`\u00`)
			buf.WriteByte(hexDigits[c>>4])
			buf.WriteByte(hexDigits[c&0xF])
		}
		start = i + 1
	}
	if start < len(s) {
		buf.WriteString(s[start:])
	}
	buf.WriteByte('"')
}
