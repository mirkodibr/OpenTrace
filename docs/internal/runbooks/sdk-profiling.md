# SDK Profiling Runbook

Day 32 performance work: baseline measurement, the serializer rewrite it
justified, an escape-analysis audit of the hot path, and a string-interning
experiment. Also documents the repeatable 4-step pprof procedure for future
investigations.

## 1. Baseline (before optimisation)

```
go test -bench=BenchmarkSerialize -benchmem -count=3 ./sdk/go/internal/exporter/...
```

The exporter's Day 25 baseline used `encoding/json` over a
`map[string]interface{}` intermediate — correct, but the map construction
and reflection-based encoding cost real allocations on every batch:

| Benchmark (100-event batch) | ns/op | B/op | allocs/op |
|---|---|---|---|
| `SerializeJSON` (encoding/json baseline) | 437,000–766,000 | ~203,500 | **4,813** |

## 2. Hand-written serialiser (the change)

`sdk/go/internal/exporter/writer.go` replaces the map-building + reflection
path with direct buffer writes: a small JSON string escaper
(`writeJSONString`), stack-allocated scratch buffers for `strconv.AppendInt`/
`AppendFloat` (no heap escape — see §3), and explicit switch-based dispatch
over `Field.Type` instead of `interface{}` boxing for every scalar. The
`Any` escape hatch still falls back to `encoding/json.Marshal` — correctness
matters more than allocations on that rare, already-documented-as-boxing
path.

**Correctness is proven by equivalence, not just performance**: both
implementations are kept (`serializeJSON` as the oracle, `serialize` as the
shipped writer) and `TestSerializeEquivalence` decodes both outputs and
asserts `reflect.DeepEqual` — covering every field type, string escaping
(quotes, backslashes, control characters, multi-byte UTF-8), nested
objects, collections, the `Any` fallback, and duplicate-key last-wins
semantics. Two *intentional* divergences (key order, HTML-escaping) are
documented on `writer.go` and don't affect decoded equivalence.

| Benchmark (100-event batch) | ns/op | B/op | allocs/op |
|---|---|---|---|
| `SerializeJSON` (baseline) | 437,000–766,000 | ~203,500 | 4,813 |
| `SerializeWritten` (hand-written) | 120,000–156,000 | ~3,214 | **100** |
| **Improvement** | **~3.5–5×** | **~63×** | **~48×** |

The remaining 100 allocs/op (one per event) is `time.Time.Format`, which
allocates its result string — inherent to formatting, not something a
hand-written writer avoids without a custom time formatter. At 100 events
per batch and one batch per `BatchInterval` (default 2s), this is
negligible background-goroutine cost; not worth the complexity of a
bespoke RFC3339 formatter for a already-48×-reduced allocation count.

Run it yourself: `go test -bench=BenchmarkSerialize -benchmem -count=3 ./sdk/go/internal/exporter/...`

## 3. Escape analysis of the hot path

```
go build -gcflags="-m=2" . 2>&1 | grep "escapes to heap"
```
(PowerShell: `... | Select-String "escapes to heap"`)

Findings, filtered to what matters:

- **`logger.go` `log()`** — the two `append(evt.Fields[...], ...)` calls are
  flagged "escapes to heap" by the *static* analysis. This is a false
  positive for our purposes: `evt.Fields` is pool-managed with pre-allocated
  capacity 16 (`pool.go`), so for any realistic field count the append
  writes into existing backing-array capacity and **no allocation happens
  at runtime**. The `TestAllocBudget_*` tests (`testing.AllocsPerRun`) are
  the ground truth here — they measure actual runtime behaviour and
  confirm 0 allocs/op, overriding the compiler's conservative static
  worst-case. No fix needed; documented so a future contributor doesn't
  "fix" a non-issue.
- **`New`, `NewNop`, `With`, `WithContext`** — every flagged escape here is
  construction-time (called once per `Logger`/child, not per log call) and
  is expected: `&Logger{...}`, `&Config{...}`, the merged fields slice in
  `With`, etc. These are exactly the budgeted allocations from ADR-005
  (1 alloc for `With`).
- **`field.go` `Object`/`StringSlice`** — the variadic/slice parameter
  escaping into the returned `Field.Interface` is the documented 1–2
  alloc/op nested-field budget (ADR-005, confirmed again in the Day 31
  serializer tests).

**Conclusion: no changes required.** The hot path's 0-alloc claims hold at
runtime; every flagged escape is either a static-analysis artifact on a
capacity-safe append or an already-budgeted construction-time cost.

## 4. String interning — implemented, benchmarked, NOT adopted

Hypothesis per the Day 32 prompt: repeated field keys (`"user_id"`,
`"request_id"`, ...) might benefit from runtime interning to avoid
re-allocating the same string. Implemented a `sync.Map`-backed `intern()`
and benchmarked four scenarios (each `-count=3`):

| Scenario | ns/op | allocs/op |
|---|---|---|
| Literal key, no intern | **0.25** | 0 |
| Literal key, with intern | 15–25 | 0 |
| Dynamic key (`string([]byte)`), no intern | 6.2–7.0 | 0 |
| Dynamic key, with intern | 48–58 | **1** |

**Verdict: not adopted, in every scenario.** Go's compiler already dedupes
identical string literals at compile time — `String("user_id", v)` costs
0.25 ns and 0 allocations with no interning at all, because the literal
`"user_id"` is a single shared constant in the binary's read-only data,
not a fresh allocation per call site. Wrapping it in `intern()` adds a
`sync.Map.Load` for zero benefit (60–100× slower). For a dynamically
constructed key, interning is *also* slower and now allocates (the
`sync.Map.Store` path) — there's no regime where it wins. The experiment
file was deleted after capturing these numbers; nothing shipped.

## 5. pprof procedure (for future investigations)

**Step 1 — CPU profile:**
```
go test -bench=BenchmarkLoggerInfo -cpuprofile=cpu.prof ./sdk/go/...
go tool pprof -http=:8080 cpu.prof
```
Web UI → Graph view → look for functions with high *self* time. Expected
hotspots on the hot path: `time.Now()`, channel send (`runtime.chansend`).
A new hotspot here that wasn't here before is the signal to investigate.

**Step 2 — Memory allocation profile:**
```
go test -bench=BenchmarkLoggerInfo -memprofile=mem.prof ./sdk/go/...
go tool pprof -alloc_objects -http=:8080 mem.prof
```
Any allocation site on the *hot path* (not construction/background paths)
above 0 objects/op is a regression against the ADR-005 budget — fail the
build, don't just note it.

**Step 3 — Heap escape analysis:**
```
go build -gcflags="-m=2" ./sdk/go/... 2>&1 | Select-String "escapes to heap"
```
Cross-reference against §3 above. Distinguish real hot-path escapes from
construction-time/static-analysis-artifact escapes using
`testing.AllocsPerRun` as ground truth — never trust the static flag alone.

**Step 4 — Trace visualisation:**
```
go test -bench=BenchmarkLoggerInfo -trace=trace.out ./sdk/go/...
go tool trace trace.out
```
Goroutine analysis view → check the background batcher/exporter goroutines
aren't blocking each other or the hot path. All commands are Windows/
PowerShell-compatible as written (pprof and trace serve local HTTP UIs
regardless of OS).
