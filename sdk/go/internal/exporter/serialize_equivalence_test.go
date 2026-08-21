package exporter

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/opentrace/opentrace-go/internal/wire"
)

// decodeGeneric decodes a serialised payload into a generic structure for
// semantic comparison (JSON object key order and number formatting are not
// semantically meaningful — see writer.go's doc comment).
func decodeGeneric(t *testing.T, b []byte) map[string]interface{} {
	t.Helper()
	var v map[string]interface{}
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, b)
	}
	return v
}

// buildEquivalenceBatch exercises every field type and every top-level
// event shape (no fields, trace/span only, mixed attrs, nested).
func buildEquivalenceBatch() []*wire.LogEvent {
	now := time.Date(2026, 1, 15, 10, 30, 0, 123456789, time.UTC)
	mk := func(msg string, fields ...wire.Field) *wire.LogEvent {
		e := wire.AcquireEvent()
		e.Level = wire.LevelWarn
		e.Message = msg
		e.Timestamp = now
		e.Fields = append(e.Fields, fields...)
		return e
	}
	return []*wire.LogEvent{
		mk("no fields at all"),
		mk("trace and span only",
			wire.String("trace_id", "0af7651916cd43dd8448eb211c80319c"),
			wire.String("span_id", "b7ad6b7169203331"),
		),
		mk("scalar mix",
			wire.String("user_id", "u_123"),
			wire.Int("count", -7),
			wire.Int64("big", 1<<40),
			wire.Float64("total", 99.95),
			wire.Bool("ok", true),
			wire.Duration("elapsed", 1500*time.Millisecond),
			wire.Err(errTestSentinel),
		),
		mk("strings needing escaping",
			wire.String("quote", `He said "hi"`),
			wire.String("backslash", `C:\path\to\file`),
			wire.String("newline", "line1\nline2\ttabbed"),
			wire.String("unicode", "héllo wörld 日本語 😀"),
			wire.String("control", "\x01\x02\x1f"),
		),
		mk("nested and collections",
			wire.Object("user",
				wire.String("id", "u_42"),
				wire.Object("permissions", wire.Bool("billing", true), wire.Bool("admin", false)),
			),
			wire.StringSlice("tags", []string{"vip", "beta", ""}),
			wire.Map("labels", map[string]string{"tier": "gold", "region": "eu"}),
		),
		mk("any values",
			wire.Any("num", 42),
			wire.Any("nested_struct", struct{ X, Y int }{1, 2}),
		),
		mk("duplicate key: last wins",
			wire.String("dup", "first"),
			wire.String("dup", "second"),
		),
	}
}

var errTestSentinel = &testError{"boom"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestSerializeEquivalence(t *testing.T) {
	res := wire.Resource{
		ServiceName: "equiv-test", ServiceVersion: "1.0.0",
		Environment: "test", Hostname: "host-1", PID: 4242,
	}

	batchA := buildEquivalenceBatch()
	batchB := buildEquivalenceBatch() // fresh events; the batch is consumed(ish) by inspection only, not released

	var bufJSON, bufFast bytes.Buffer
	if err := serializeJSON(&bufJSON, batchA, res); err != nil {
		t.Fatalf("serializeJSON: %v", err)
	}
	if err := serialize(&bufFast, batchB, res); err != nil {
		t.Fatalf("serialize (hand-written): %v", err)
	}

	gotJSON := decodeGeneric(t, bufJSON.Bytes())
	gotFast := decodeGeneric(t, bufFast.Bytes())

	if !reflect.DeepEqual(gotJSON, gotFast) {
		t.Fatalf("outputs are not semantically equivalent:\nJSON baseline: %s\nhand-written:  %s",
			bufJSON.String(), bufFast.String())
	}

	// Spot-check the duplicate-key last-wins behaviour explicitly, since
	// it depends on the decoder's semantics rather than the writer's.
	events := gotFast["events"].([]interface{})
	dupEvent := events[len(events)-1].(map[string]interface{})
	attrs := dupEvent["log_attributes"].(map[string]interface{})
	if attrs["dup"] != "second" {
		t.Errorf("duplicate key did not resolve to last-value-wins: %v", attrs["dup"])
	}
}

func TestSerializeEquivalence_EmptyBatch(t *testing.T) {
	res := wire.Resource{ServiceName: "svc"}
	var bufJSON, bufFast bytes.Buffer
	_ = serializeJSON(&bufJSON, nil, res)
	_ = serialize(&bufFast, nil, res)
	gotJSON := decodeGeneric(t, bufJSON.Bytes())
	gotFast := decodeGeneric(t, bufFast.Bytes())
	if !reflect.DeepEqual(gotJSON, gotFast) {
		t.Fatalf("empty batch mismatch: %s vs %s", bufJSON.String(), bufFast.String())
	}
}

func TestWriteJSONString_ControlCharacters(t *testing.T) {
	var buf bytes.Buffer
	writeJSONString(&buf, "a\x00b\x1fc\"d\\e\nf\tg\rh")
	var decoded string
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("escaped output is not valid JSON: %v (%s)", err, buf.String())
	}
	want := "a\x00b\x1fc\"d\\e\nf\tg\rh"
	if decoded != want {
		t.Fatalf("round-trip mismatch: got %q, want %q", decoded, want)
	}
}
