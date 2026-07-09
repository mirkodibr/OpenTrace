package opentrace

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// newTestLogger builds a Logger with a bare buffer and no background
// pipeline, so tests can inspect enqueued events directly via drainBuffer.
func newTestLogger(level Level, bufferCap int) *Logger {
	return &Logger{
		level:   level,
		buffer:  newEventBuffer(bufferCap),
		res:     resourceInfo{ServiceName: "test-service"},
		dropped: &atomic.Int64{},
		closed:  &atomic.Bool{},
	}
}

// drainBuffer empties the logger's buffer and returns the events in order.
func drainBuffer(l *Logger) []*LogEvent {
	var events []*LogEvent
	for {
		select {
		case e := <-l.buffer.ch:
			events = append(events, e)
		default:
			return events
		}
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	tests := []struct {
		name     string
		minLevel Level
		emit     func(l *Logger)
		want     int
	}{
		{"debug below info is filtered", LevelInfo, func(l *Logger) { l.Debug("d") }, 0},
		{"info at info passes", LevelInfo, func(l *Logger) { l.Info("i") }, 1},
		{"warn above info passes", LevelInfo, func(l *Logger) { l.Warn("w") }, 1},
		{"error below fatal is filtered", LevelFatal, func(l *Logger) { l.Error("e") }, 0},
		{"debug at debug passes", LevelDebug, func(l *Logger) { l.Debug("d") }, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newTestLogger(tt.minLevel, 8)
			tt.emit(l)
			if got := len(drainBuffer(l)); got != tt.want {
				t.Fatalf("got %d events in buffer, want %d", got, tt.want)
			}
		})
	}
}

func TestLoggerSetsTimestampAndLevel(t *testing.T) {
	l := newTestLogger(LevelInfo, 8)
	before := time.Now()
	l.Warn("something happened", String("k", "v"))
	after := time.Now()

	events := drainBuffer(l)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	e := events[0]
	if e.Level != LevelWarn {
		t.Errorf("Level = %v, want %v", e.Level, LevelWarn)
	}
	if e.Message != "something happened" {
		t.Errorf("Message = %q", e.Message)
	}
	if e.Timestamp.Before(before) || e.Timestamp.After(after) {
		t.Errorf("Timestamp %v outside [%v, %v]", e.Timestamp, before, after)
	}
	if len(e.Fields) != 1 || e.Fields[0].Key != "k" || e.Fields[0].StringVal != "v" {
		t.Errorf("Fields = %+v, want single String(k, v)", e.Fields)
	}
}

func TestLoggerWith(t *testing.T) {
	parent := newTestLogger(LevelInfo, 8)
	child := parent.With(String("request_id", "req-1"), Int("attempt", 2))

	// Child events carry parent fields plus call-site fields, in order.
	child.Info("child event", Bool("ok", true))
	events := drainBuffer(parent) // shared buffer
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	keys := make([]string, 0, 3)
	for _, f := range events[0].Fields {
		keys = append(keys, f.Key)
	}
	want := []string{"request_id", "attempt", "ok"}
	if len(keys) != len(want) {
		t.Fatalf("field keys = %v, want %v", keys, want)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("field keys = %v, want %v", keys, want)
		}
	}

	// Parent is unmodified: its own events carry no pre-set fields.
	parent.Info("parent event")
	events = drainBuffer(parent)
	if len(events) != 1 || len(events[0].Fields) != 0 {
		t.Fatalf("parent logger gained fields: %+v", events[0].Fields)
	}

	// Grandchild stacks fields on top of child.
	grandchild := child.With(String("stage", "commit"))
	grandchild.Info("gc event")
	events = drainBuffer(parent)
	if len(events) != 1 || len(events[0].Fields) != 3 {
		t.Fatalf("grandchild fields = %+v, want 3 fields", events[0].Fields)
	}
}

func TestLoggerWithContext(t *testing.T) {
	l := newTestLogger(LevelInfo, 8)

	// Context without trace info: no extra fields.
	plain := l.WithContext(context.Background())
	plain.Info("no trace")
	events := drainBuffer(l)
	if len(events) != 1 || len(events[0].Fields) != 0 {
		t.Fatalf("expected no fields without trace context, got %+v", events[0].Fields)
	}

	// Context carrying trace IDs via ContextWithTrace.
	ctx := ContextWithTrace(context.Background(), "0af7651916cd43dd8448eb211c80319c", "b7ad6b7169203331")
	traced := l.WithContext(ctx)
	traced.Info("traced")
	events = drainBuffer(l)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	fields := events[0].Fields
	if len(fields) != 2 {
		t.Fatalf("fields = %+v, want trace_id and span_id", fields)
	}
	if fields[0].Key != "trace_id" || fields[0].StringVal != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("trace_id field = %+v", fields[0])
	}
	if fields[1].Key != "span_id" || fields[1].StringVal != "b7ad6b7169203331" {
		t.Errorf("span_id field = %+v", fields[1])
	}
}

func TestNopLogger(t *testing.T) {
	l := NewNop()
	// All methods must be callable without error or effect.
	l.Debug("d")
	l.Info("i", String("k", "v"))
	l.Warn("w")
	l.Error("e", Err(errors.New("boom")))
	child := l.With(String("k", "v"))
	child.Info("via child")
	if got := len(drainBuffer(l)); got != 0 {
		t.Fatalf("nop logger enqueued %d events, want 0", got)
	}
	if l.DroppedCount() != 0 {
		t.Fatalf("nop logger dropped count = %d, want 0", l.DroppedCount())
	}
	if err := l.Shutdown(context.Background()); err != nil {
		t.Fatalf("nop Shutdown returned %v", err)
	}
}

func TestDroppedCounter(t *testing.T) {
	l := newTestLogger(LevelInfo, 2)
	l.Info("1")
	l.Info("2")
	l.Info("3") // buffer full — dropped
	l.Info("4") // dropped
	if got := l.DroppedCount(); got != 2 {
		t.Fatalf("DroppedCount = %d, want 2", got)
	}
	if got := len(drainBuffer(l)); got != 2 {
		t.Fatalf("buffer holds %d events, want 2", got)
	}
}

func TestLoggerAfterShutdownDrops(t *testing.T) {
	l := newTestLogger(LevelInfo, 8)
	if err := l.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	l.Info("after shutdown")
	if got := l.DroppedCount(); got != 1 {
		t.Fatalf("DroppedCount = %d, want 1", got)
	}
	if got := len(drainBuffer(l)); got != 0 {
		t.Fatalf("buffer holds %d events after shutdown, want 0", got)
	}
	// Shutdown is idempotent.
	if err := l.Shutdown(context.Background()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestPoolResetSemantics(t *testing.T) {
	e := acquireEvent()
	e.Level = LevelError
	e.Message = "sensitive payload"
	e.Timestamp = time.Now()
	e.Fields = append(e.Fields, String("password", "hunter2"), Int("n", 42))
	capBefore := cap(e.Fields)

	releaseEvent(e)

	// The same object comes back from the pool in a clean state. sync.Pool
	// gives no guarantee we get the same pointer, so assert on the released
	// object directly — releaseEvent must have scrubbed it already.
	if e.Message != "" {
		t.Errorf("Message not reset: %q", e.Message)
	}
	if e.Level != 0 {
		t.Errorf("Level not reset: %v", e.Level)
	}
	if !e.Timestamp.IsZero() {
		t.Errorf("Timestamp not reset: %v", e.Timestamp)
	}
	if len(e.Fields) != 0 {
		t.Errorf("Fields length not reset: %d", len(e.Fields))
	}
	if cap(e.Fields) != capBefore {
		t.Errorf("Fields backing array not retained: cap %d, want %d", cap(e.Fields), capBefore)
	}
	// Data-leak check: the backing array slots must be zeroed.
	spare := e.Fields[:cap(e.Fields)]
	for i, f := range spare[:2] {
		if f.Key != "" || f.StringVal != "" || f.Int64Val != 0 {
			t.Errorf("field slot %d retains data after release: %+v", i, f)
		}
	}
}

func TestNewValidatesBeforeStartingAnything(t *testing.T) {
	// Missing endpoint + service name → error, no logger.
	l, err := New()
	if err == nil {
		t.Fatal("New() with empty config succeeded, want validation error")
	}
	if l != nil {
		t.Fatal("New() returned a logger alongside an error")
	}
}
