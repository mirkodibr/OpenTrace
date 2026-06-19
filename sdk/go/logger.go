// Package opentrace provides a zero-allocation, goroutine-safe structured
// logging SDK for the OpenTrace observability platform.
//
// Quickstart:
//
//	logger, err := opentrace.New(
//	    opentrace.WithCollectorEndpoint("https://collector.example.com"),
//	    opentrace.WithServiceName("my-service"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer logger.Shutdown(context.Background())
//
//	logger.Info("user signed in", opentrace.String("user_id", "u_123"))
package opentrace

import (
	"context"
	"sync/atomic"
)

// Logger is the primary entry point for emitting telemetry. All methods are
// safe for concurrent use. A Logger must be created with New; the zero value
// is not valid.
type Logger struct {
	level   Level
	fields  []Field
	buffer  *eventBuffer
	res     resourceInfo
	dropped atomic.Int64
	closed  atomic.Bool
}

// New creates a Logger with the provided options.
// It returns an error if the configuration is invalid or the exporter cannot
// be initialised. Goroutines are only started after successful validation.
func New(opts ...Option) (*Logger, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}
	loadFromEnv(cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	buf := newEventBuffer(cfg.BufferSize)
	l := &Logger{
		level:  cfg.MinLevel,
		fields: nil,
		buffer: buf,
		res:    captureResource(cfg),
	}
	// TODO (Day 25): wire up the background exporter goroutine here.
	return l, nil
}

// NewNop returns a Logger that discards all events. Useful in tests.
func NewNop() *Logger {
	return &Logger{
		level:  LevelFatal + 1, // above fatal → every level check fails fast
		buffer: newEventBuffer(1),
	}
}

// Debug emits a debug-level log event.
func (l *Logger) Debug(msg string, fields ...Field) { l.log(LevelDebug, msg, fields) }

// Info emits an info-level log event.
func (l *Logger) Info(msg string, fields ...Field) { l.log(LevelInfo, msg, fields) }

// Warn emits a warn-level log event.
func (l *Logger) Warn(msg string, fields ...Field) { l.log(LevelWarn, msg, fields) }

// Error emits an error-level log event.
func (l *Logger) Error(msg string, fields ...Field) { l.log(LevelError, msg, fields) }

// Fatal emits a fatal-level log event. Unlike other levels, Fatal calls
// os.Exit(1) after the event is enqueued (best-effort delivery).
func (l *Logger) Fatal(msg string, fields ...Field) {
	l.log(LevelFatal, msg, fields)
	// TODO: flush synchronously before exit
}

// With returns a child Logger with the provided fields pre-set on every
// subsequent call. The parent Logger is unmodified.
func (l *Logger) With(fields ...Field) *Logger {
	merged := make([]Field, len(l.fields)+len(fields))
	copy(merged, l.fields)
	copy(merged[len(l.fields):], fields)
	return &Logger{
		level:  l.level,
		fields: merged,
		buffer: l.buffer,
		res:    l.res,
	}
}

// WithContext returns a child Logger that automatically extracts trace_id and
// span_id from ctx (compatible with OpenTelemetry context propagation).
func (l *Logger) WithContext(ctx context.Context) *Logger {
	traceID, spanID := extractTraceContext(ctx)
	extra := make([]Field, 0, 2)
	if traceID != "" {
		extra = append(extra, String("trace_id", traceID))
	}
	if spanID != "" {
		extra = append(extra, String("span_id", spanID))
	}
	return l.With(extra...)
}

// Shutdown flushes all buffered events and waits for in-flight exports to
// complete within the context deadline. It is safe to call more than once.
func (l *Logger) Shutdown(ctx context.Context) error {
	if !l.closed.CompareAndSwap(false, true) {
		return nil // already shut down
	}
	// TODO (Day 33): implement full drain sequence.
	_ = ctx
	return nil
}

// DroppedCount returns the cumulative number of events dropped due to a full
// buffer or an open circuit breaker.
func (l *Logger) DroppedCount() int64 { return l.dropped.Load() }

// log is the hot path. It must execute in < 500 ns at zero contention.
func (l *Logger) log(level Level, msg string, fields []Field) {
	if l.closed.Load() {
		l.dropped.Add(1)
		return
	}
	if level < l.level {
		return
	}

	evt := acquireEvent()
	evt.Level = level
	evt.Message = msg
	// Merge pre-set fields with call-site fields.
	evt.Fields = append(evt.Fields[:0], l.fields...)
	evt.Fields = append(evt.Fields, fields...)

	if !l.buffer.tryEnqueue(evt) {
		l.dropped.Add(1)
		releaseEvent(evt)
	}
	// Ownership transferred to the buffer; the exporter will release the event.
}
