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
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// Logger is the primary entry point for emitting telemetry. All methods are
// safe for concurrent use. A Logger must be created with New; the zero value
// is not valid.
type Logger struct {
	level           Level
	fields          []Field
	buffer          *eventBuffer
	res             resourceInfo
	pipe            *pipeline // nil for nop/test loggers; shared by all children
	dropped         *atomic.Int64
	closed          *atomic.Bool
	shutdownTimeout time.Duration
}

// New creates a Logger with the provided options.
// It returns an error if the configuration is invalid or the exporter cannot
// be initialised. Goroutines are only started after successful validation.
func New(opts ...Option) (*Logger, error) {
	// Precedence (lowest to highest): defaults → environment → options.
	// Environment is applied before options so an explicit programmatic
	// option always beats an ambient env var.
	cfg := defaultConfig()
	loadFromEnv(cfg)
	for _, o := range opts {
		o(cfg)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	buf := newEventBuffer(cfg.BufferSize)
	l := &Logger{
		level:           cfg.MinLevel,
		fields:          nil,
		buffer:          buf,
		res:             captureResource(cfg),
		dropped:         &atomic.Int64{},
		closed:          &atomic.Bool{},
		shutdownTimeout: cfg.ShutdownTimeout,
	}
	l.pipe = newPipeline(cfg, buf, l.res, l.dropped)
	l.pipe.start()
	return l, nil
}

// NewNop returns a Logger that discards all events. Useful in tests.
func NewNop() *Logger {
	return &Logger{
		level:   LevelFatal + 1, // above fatal → every level check fails fast
		buffer:  newEventBuffer(1),
		dropped: &atomic.Int64{},
		closed:  &atomic.Bool{},
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

// Fatal emits a fatal-level log event, makes a best-effort attempt to flush
// it (3-second cap), and then calls os.Exit(1).
func (l *Logger) Fatal(msg string, fields ...Field) {
	l.log(LevelFatal, msg, fields)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	_ = l.Shutdown(ctx)
	cancel()
	osExit(1)
}

// osExit is swappable so tests can observe Fatal without killing the process.
var osExit = os.Exit

// With returns a child Logger with the provided fields pre-set on every
// subsequent call. The parent Logger is unmodified.
func (l *Logger) With(fields ...Field) *Logger {
	merged := make([]Field, len(l.fields)+len(fields))
	copy(merged, l.fields)
	copy(merged[len(l.fields):], fields)
	return &Logger{
		level:           l.level,
		fields:          merged,
		buffer:          l.buffer,
		res:             l.res,
		pipe:            l.pipe,
		dropped:         l.dropped,
		closed:          l.closed,
		shutdownTimeout: l.shutdownTimeout,
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

// Shutdown gates all producers, flushes buffered events, and waits for
// in-flight exports to complete within the context deadline. It is
// idempotent; concurrent and repeated calls share the first call's result.
//
// If ctx carries no deadline, Shutdown applies WithShutdownTimeout's value
// (default 15s) itself, so logger.Shutdown(context.Background()) still
// returns promptly rather than blocking indefinitely if the collector is
// unreachable. Pass a context.WithTimeout explicitly to use a different
// bound for one call.
func (l *Logger) Shutdown(ctx context.Context) error {
	l.closed.Store(true) // gate producers before draining (ADR-005 ordering)
	if l.pipe == nil {
		return nil // nop/test logger — nothing to drain
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, l.shutdownTimeout)
		defer cancel()
	}
	return l.pipe.stop(ctx)
}

// RegisterSignalHandler installs a SIGINT/SIGTERM handler that calls
// Shutdown (bounded by WithShutdownTimeout) when the process receives a
// termination signal, then restores the default signal behaviour so a
// second Ctrl-C forces an immediate exit. It returns a stop function the
// caller may use to deregister the handler early (mainly useful in tests).
//
// This is entirely optional: applications that already own a signal
// handler and a shutdown sequence should call logger.Shutdown(ctx) directly
// from their own handler instead of using this method — installing two
// handlers is harmless (both call the idempotent Shutdown) but redundant.
func (l *Logger) RegisterSignalHandler() (stop func()) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), l.shutdownTimeout)
		defer shutdownCancel()
		_ = l.Shutdown(shutdownCtx)
	}()
	return cancel
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
	evt.Timestamp = time.Now()
	// Merge pre-set fields with call-site fields.
	evt.Fields = append(evt.Fields[:0], l.fields...)
	evt.Fields = append(evt.Fields, fields...)

	if !l.buffer.tryEnqueue(evt) {
		l.dropped.Add(1)
		releaseEvent(evt)
	}
	// Ownership transferred to the buffer; the exporter will release the event.
}
