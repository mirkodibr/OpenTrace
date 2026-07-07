# ADR-005: Go SDK Architecture and Allocation Budget

**Status:** Accepted
**Date:** Phase 2, Day 22

## Context

The OpenTrace Go SDK (`github.com/opentrace/opentrace-go`) is embedded in the critical
path of host applications. Any latency, allocation, or blocking behaviour it introduces
degrades the host directly. The design targets, in order of priority:

1. **< 5 µs overhead per log call** on the hot path (< 500 ns at zero contention).
2. **Zero heap allocations per log call** at steady state for scalar field types.
3. **Never block the calling goroutine** for network I/O — drop rather than block.
4. **Safe for concurrent use** from any number of goroutines without external locking.

This ADR fixes the public API contract, the internal layering, and the allocation
budget before the pipeline components (batcher, exporter, retry) are implemented.

## Public API Contract

The following surface is the v1 contract. Additions are allowed; removals or signature
changes require a major version bump.

```go
// Construction
func New(opts ...Option) (*Logger, error)   // validates config before spawning goroutines
func NewNop() *Logger                       // no-op logger for tests

// Emission (hot path — allocation-free for scalar fields)
func (l *Logger) Debug(msg string, fields ...Field)
func (l *Logger) Info(msg string, fields ...Field)
func (l *Logger) Warn(msg string, fields ...Field)
func (l *Logger) Error(msg string, fields ...Field)
func (l *Logger) Fatal(msg string, fields ...Field)  // best-effort flush, then os.Exit(1)

// Derivation
func (l *Logger) With(fields ...Field) *Logger        // child logger, pre-set fields
func (l *Logger) WithContext(ctx context.Context) *Logger // extracts trace_id/span_id

// Lifecycle & self-telemetry
func (l *Logger) Shutdown(ctx context.Context) error  // ordered drain, idempotent
func (l *Logger) DroppedCount() int64                 // cumulative dropped events

// Field constructors
String, Int, Int64, Float64, Bool, Duration  // zero allocations
Err(error)                                   // one allocation (interface boxing)
Any(key, interface{})                        // one allocation (escape hatch)

// Configuration: functional options (WithCollectorEndpoint, WithServiceName, …)
// with environment-variable fallback (OPENTRACE_*) and multi-error validation.
```

## Three-Layer Architecture

Each layer owns its state exclusively; hand-off between layers happens through
channels, which provide the synchronisation.

```
┌─ Layer 1: Application API (any goroutine) ─────────────────────────────┐
│ logger.Info() → level check → acquire *LogEvent from sync.Pool →      │
│ populate → non-blocking channel send. Full buffer = drop + counter.   │
└──────────────────────────────┬─────────────────────────────────────────┘
                               │ eventBuffer (buffered chan *LogEvent)
┌─ Layer 2: Batcher (1 goroutine) ──────────────────────────────────────┐
│ Drains the buffer, accumulates a batch, flushes on ANY of:            │
│ size ≥ maxSize │ est. bytes ≥ maxBytes │ ticker ≥ maxInterval.        │
│ Flush = slice swap + non-blocking send to flushCh (full = drop).      │
└──────────────────────────────┬─────────────────────────────────────────┘
                               │ flushCh (buffered chan []*LogEvent)
┌─ Layer 3: Exporter (1 goroutine) ─────────────────────────────────────┐
│ Serialises batch to the collector wire format, releases events back   │
│ to the pool, gzips > 1 KB, POSTs with retry + circuit breaker.        │
└─────────────────────────────────────────────────────────────────────────┘
```

Exactly **two** background goroutines exist per Logger, both owned by an unexported
`pipeline` struct and tracked by a single `sync.WaitGroup`. Child loggers created by
`With`/`WithContext` share the parent's buffer and pipeline — they spawn nothing.

### D1 — Pipeline wiring and goroutine ownership

`New()` constructs buffer → batcher → flushCh → exporter and starts both goroutines
only after `Config.Validate()` passes. The `pipeline` struct is the single owner of
goroutine lifecycle: it starts them, and `Shutdown` stops them in order
(batcher first, then exporter). No other component may spawn goroutines.

### D2 — Event pool ownership and the release point

`*LogEvent` objects are recycled through a `sync.Pool`. Ownership passes linearly:

```
log() ── enqueue ok ──→ buffer ──→ batcher ──→ exporter ── serialise ──→ release
   └── enqueue failed ──→ release (drop)
                          batcher flushCh full ──→ release (drop)
```

The exporter serialises the batch to bytes **first** and releases every event
**before** any network I/O. Retries operate on the serialised bytes only. This bounds
retry memory and makes the release point unambiguous — an event is never touched
after it has been handed to `releaseEvent`.

### D3 — Internal package layout

`LogEvent`, `Field`, `Level`, and the pool move to `sdk/go/internal/wire/`. The root
`opentrace` package re-exports them via type aliases (`type Field = wire.Field`) and
thin constructor wrappers that inline to zero overhead. This lets
`internal/batcher`, `internal/exporter`, and `internal/retry` import the shared types
without an import cycle back into the root package. `internal/retry` is fully generic
(`func(context.Context) error`) and imports no wire types at all.

### D4 — SDK-side field flattening

The SDK serialiser emits **exactly** the collector wire format defined in
`pkg/schema/log.go`: fields are flattened into the `log_attributes` JSON map, and
`trace_id`/`span_id` fields are lifted to top-level columns during serialisation.
Rationale: (a) the collector stays SDK-agnostic — any client speaking the documented
schema works; (b) no second deserialise/re-serialise hop in the collector hot path;
(c) *sanitisation* (attribute depth/key limits) still happens collector-side, because
the collector cannot trust any client. See ADR-006 for the field-by-field mapping.

## Channel vs. Ring Buffer

| Criterion | Buffered channel (chosen) | Lock-free MPSC ring buffer |
|---|---|---|
| Throughput, single producer | ~20–30 M sends/s | ~50–100 M ops/s |
| Throughput, 100 producers | ~5–10 M sends/s (runtime futex) | ~20–40 M ops/s (CAS contention) |
| Allocations per send | 0 (pointer into pre-sized hchan) | 0 |
| Full behaviour | `select`/`default` → drop-newest, trivially correct | drop-oldest or drop-newest, subtle |
| Correctness risk | none — runtime-verified | high — memory-ordering bugs are silent |
| Lines of code | ~25 | ~200 + fuzz/stress tests |

**Decision: buffered channel.** At the design target (≤ 1 M events/s per process) the
channel is nowhere near saturation, and `select { case ch <- e: default: }` gives an
exact, race-free drop-newest policy in three lines. The escalation trigger is measured,
not speculative: if Day 34 load tests show `BenchmarkSDK_Parallel_100` throughput
collapsing below ~5 M calls/s aggregate with the channel send dominating the CPU
profile (`runtime.chansend` > 30% self time), an MPSC ring buffer becomes justified.
Until that evidence exists, the channel stays.

**Drop policy: drop-newest.** When the buffer is full the *incoming* event is dropped
and counted. Drop-oldest requires a consumer-side eviction (extra channel op per drop)
and makes loss ordering harder to reason about. Under sustained overload both policies
lose data; the dropped counter is the operator signal either way.

## Context Propagation

`WithContext(ctx)` extracts OpenTelemetry trace/span IDs **without importing the OTel
SDK**, by interface-asserting the value stored under OTel's context key against a
minimal `interface { SpanContext() ... }` shape (see `resource.go`). If the context
carries no recognisable span, the child logger simply gains no extra fields. This
keeps the SDK dependency-free while remaining OTel-compatible.

## Allocation Budget (enforced, not aspirational)

| Operation | Budget | Enforcement |
|---|---|---|
| `Info(msg)` — no fields | 0 allocs / 0 B | `testing.AllocsPerRun` + benchmark |
| `Info(msg, String/Int/Bool/Duration...)` | 0 allocs / 0 B | `testing.AllocsPerRun` + benchmark |
| `Info(msg, Any(k, v))` | 1 alloc (interface boxing) | benchmark |
| `Err(err)` | 1 alloc (interface boxing) | benchmark |
| `With(fields...)` | 1 alloc (child Logger) + 1 (merged slice) | benchmark |
| Event below MinLevel | 0 allocs — early return before pool | `AllocsPerRun` |
| Batch flush (N events) | bounded: 1 slice swap + serialisation buffer (pooled) | benchmark `BenchmarkSerialize100Events` |

Budget tests live behind `//go:build !race` (allocation counts are unreliable under
the race detector) and must run in a non-race CI step. A regression in any 0-alloc
row is a build failure, not a code-review discussion.

### Zero-allocation techniques required

- `Field` is a **value struct** with a union-like layout (one slot per scalar kind),
  never an interface — interfaces box on assignment.
- `*LogEvent` recycling via `sync.Pool` with a pre-sized `Fields []Field` (cap 16);
  release resets `len` to 0 but keeps the backing array.
- No `encoding/json` (reflection) anywhere on the hot path — serialisation happens
  only in the exporter goroutine.
- `log()` copies the variadic fields slice into the pooled event and never retains
  the argument slice, so escape analysis keeps the variadic on the caller's stack.

## Data Loss Policy (summary)

The SDK deliberately drops telemetry in four places rather than block the host:
buffer full (drop-newest), flushCh full (drop batch), circuit breaker open (drop
batch), retries exhausted (drop batch). Every drop increments the counter exposed by
`DroppedCount()`. A 1-second block inside `logger.Info()` in a web handler adds one
full second to that request's latency — losing telemetry is always preferable to
degrading the host. The full policy with monitoring guidance is documented in the
`internal/retry` package (Day 26).

## Consequences

- The batcher/exporter/retry implementations (Days 24–26) must conform to the
  layering and ownership rules above; deviations require updating this ADR.
- The wire types move to `internal/wire` (Day 24) — a pure-mechanical refactor that
  must not change the public API or the allocation profile.
- Shutdown ordering (Day 33) is constrained by D1/D2: producers are gated by an
  atomic flag, the multi-producer buffer channel is never closed, and only the
  batcher (sole sender) closes flushCh.
