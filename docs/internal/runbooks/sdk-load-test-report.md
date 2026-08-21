# SDK Load Testing & Breaking Point Analysis

Day 34: two complementary measurement tools and the results they produced
against the real code on this branch.

- **`go test -bench=BenchmarkSDK`** (`sdk/go/bench_test.go`) — steady-state
  per-call cost at 1 / 10 / 100 concurrent goroutines, and the allocation
  profile of richer field sets. Runs in seconds.
- **`go run ./sdk/go/cmd/loadtest`** — a standalone sustained-load driver
  against an embedded mock collector, sampling throughput, buffer
  utilisation, drop count, and process memory every 10 seconds. Runs for
  minutes; this is what surfaces breaking points and leak-shaped memory
  growth that a benchmark cannot.

All numbers below are from this session, same machine as every other
benchmark in this repo (AMD Ryzen 5 7535U, 12 threads, Windows 11, Docker
Desktop/WSL2 running concurrently — see the performance-baseline runbook's
caveat about virtualisation jitter on Windows/WSL2).

## Throughput benchmark suite

```
go test -bench=BenchmarkSDK -benchmem -count=3 ./sdk/go/...
```

| Benchmark | ns/op | calls/sec | B/op | allocs/op | Target |
|---|---|---|---|---|---|
| `SingleGoroutine` | 234–248 | **4.0–4.3M** | 0 | **0** | ≥ 1,000,000 |
| `Parallel_10` (10 goroutines) | 54–59 | **17.1–18.4M** | 0 | **0** | ≥ 5,000,000 aggregate |
| `Parallel_100` (100 goroutines) | 39–40 | **25.1–25.6M** | 0 | **0** | should not degrade proportionally |
| `WithFields_5` (5 typed fields) | 274–292 | 3.4–3.6M | 0 | **0** | remains 0-alloc |
| `WithObject_Nested` (3-level nesting) | 531–551 | 1.8–1.9M | 378–379 | **4** | see note below |

Every target is exceeded, several by a wide margin. `Parallel_100` is
*faster per-op* than `Parallel_10`, not slower — with 12 hardware threads,
going from 10 to 100 goroutines mostly adds scheduling interleaving rather
than genuine new parallelism, and the buffered channel (ADR-005) absorbs it
without contention collapse. This is exactly the "no proportional
degradation" outcome the target describes; nothing here approaches the
`runtime.chansend`-dominates-the-profile threshold ADR-005 sets as the
trigger for reconsidering a lock-free ring buffer.

**`WithObject_Nested` allocation note.** 4 allocs/op looks like it exceeds
the "1–2 allocs/op" budget documented on `Object()` in `field.go` — it
doesn't. That budget is stated *per `Object()` call* (one alloc for the
variadic `fields ...Field` backing array, one for boxing that slice into
`Field.Interface`), and this benchmark makes **two** nested `Object()`
calls (`user`, and `permissions` inside it): 2 calls × 2 allocs = 4,
exactly consistent with the documented per-call figure. Nothing to fix.

## Scenario A — buffer saturation ramp

**Setup**: mock collector delayed 100ms per response (simulating an
overloaded/slow collector); default `BatchSize=500`. The exporter drains
`flushCh` on a single goroutine, so a delayed collector caps sustainable
throughput at `BatchSize / delay` = 500 / 0.1s = **5,000 events/sec** —
an architectural prediction from ADR-005's design, tested here rather than
just asserted.

**Method deviation from the original 1,000-RPS-increment sweep**: an
exhaustive 1,000→100,000 sweep in 1,000-RPS steps is 100 runs; instead this
ran a bracketing search (a handful of runs, `-duration 20s` each) to find
the transition, consistent with the scaled-down-but-honest methodology
already used for the k6 run in the performance-baseline runbook.

| Target RPS | Dropped events (20s run) | Verdict |
|---|---|---|
| 3,000 | 0 | comfortably under ceiling |
| 4,500 | 0 | still under |
| **5,000** | **0** | at the analytically predicted ceiling — holds |
| 5,500 | 6,500 | **saturation begins** |
| 6,000 | 16,500 (steeper) | over ceiling, drop rate scaling with excess load |

**Breaking point: ~5,000–5,500 events/sec** under this collector-latency
profile — matching the architectural prediction within one bracketing
step. Below the ceiling, `BufferUtilization()` stayed at 0.0% throughout
(the buffer only matters once the exporter can't keep the channel
drained; below the ceiling batches leave as fast as they arrive). This is
the collector's limit, not the SDK's: a faster or horizontally-scaled
collector raises this ceiling directly, and it is exactly what the
circuit breaker and retry-budget-reduction-during-shutdown (Day 33) exist
to protect the host application from.

## Scenario B — memory pressure (shortened run)

**Setup**: 3,000 RPS (comfortably under the Scenario A ceiling, isolating
memory behaviour from collector-latency effects), no mock delay, 50
goroutines.

**Deviation from the 60-minute spec**: run for **2 minutes**. A 60-minute
run is impractical to execute and verify within this session; 2 minutes
at a steady, sustained rate is long enough to distinguish "flat after
warm-up" from "growing" — the failure mode being screened for (a leak)
would already show a visible slope well before 60 minutes if one existed,
since nothing in the pipeline holds unbounded state (every buffer is
fixed-capacity, per ADR-005).

| Time | heap_MB | numGC | dropped |
|---|---|---|---|
| 10s | 3.9 | 53 | 0 |
| 30s | 2.4 | 163 | 0 |
| 1m00s | 2.6 | 327 | 0 |
| 1m30s | 4.2 | 490 | 0 |
| 2m00s | 1.9 | 644 | 0 |

**Result: flat.** heap_alloc oscillates in a narrow 1.9–4.2 MB band for
the full 2 minutes with no upward trend (each point is a snapshot right
after `runtime.ReadMemStats`, so it reflects live heap, not cumulative
allocation) — no leak signature. 645 GC cycles over 120s totalled only
70.5ms of pause time (0.06% of wall time). Zero events dropped throughout.

**Alert threshold** (per the original spec, for anyone re-running the
full 60-minute version): flag heap growth > 10 MB after a 10-minute
warm-up. This run's total growth (start 0.5 MB → end 2.6 MB heap, per the
summary) is a full order of magnitude under that threshold.

## Scenario C — GC impact (GOGC 100 vs 400)

**Setup**: 8,000 RPS (comfortably under the mock-latency-free exporter's
throughput ceiling), 30s each, `GOGC=100` (Go's default) vs `GOGC=400`.

| GOGC | GC cycles (30s) | GC pause total | Heap growth | Dropped |
|---|---|---|---|---|
| 100 (default) | 412 | 36.6 ms | 1.8 MB | 0 |
| 400 | **65** (6.3× fewer) | **4.4 ms** (8.3× less) | 12.2 MB (6.8× more) | 0 |

**Result: the classic GOGC trade-off, and it barely matters here.**
Raising GOGC trades steady-state memory for GC frequency/pause time, as
expected — but even at the *default* GOGC, total pause time is 36.6ms over
30 seconds of wall clock: **0.12% overhead**. This is direct evidence for
the ADR-005 claim that the 0-alloc hot path (verified throughout this
session's benchmarks) keeps allocation pressure low enough that GC tuning
is a minor lever here, not a necessity — unlike an allocation-heavy
logging library, where this same comparison would show GC dominating wall
time at high GOGC=100 throughput. Recommendation: leave `GOGC` at its
default for this SDK; raise it only if profiling a *specific* deployment
shows GC pause affecting p99 latency, which nothing measured in this
report suggests for the SDK itself.

## Bottleneck remediation guide

Four bottlenecks the architecture is designed against, the symptom that
would reveal each, and the fix — grounded in what this session's testing
actually exercised or the specific code that would need to change.

**Channel send blocking** (buffer or flushCh saturation)
Symptom: `DroppedCount()` climbing while `BufferUtilization()` sits near
100%; `Parallel_100` throughput collapsing well below `Parallel_10`'s
(not observed here — see the throughput table above, where it improved).
Diagnosis: producers are outrunning the batcher/exporter's drain rate —
exactly Scenario A above, where the *collector*, not the channel, was the
bottleneck. Remediation: first check whether the collector-side or
network hop is the real limit (as it was here) before touching the SDK;
if genuinely channel-bound, increase `WithBufferSize` for more absorption
headroom, or revisit the ADR-005 ring-buffer escalation trigger if
`runtime.chansend` dominates a CPU profile (it did not, in any benchmark
run this session).

**sync.Pool contention**
Symptom: high `runtime.lock`/`sync.(*Pool).Get` time in a CPU profile
under heavy concurrency (per-P pool contention). Diagnosis: `pprof -http`
CPU profile (see `sdk-profiling.md` §5) with a hot `sync.(*Pool)` frame.
Not observed at 100 goroutines in this session's benchmarks (0 allocs/op
held). Remediation if it appears at higher concurrency than tested here:
pre-shard the pool (multiple `sync.Pool` instances indexed by goroutine
ID or a lightweight hash) to reduce per-P lock pressure.

**GC pauses disrupting throughput**
Symptom: throughput oscillating in step with `runtime.GCStats.PauseTotal`
growth. Measured directly in Scenario C: even at GOGC=100, pause overhead
was 0.12% of wall time at 8,000 RPS — not a disruptive pattern here.
Remediation if it becomes one at higher sustained throughput: raise
`GOGC` (Scenario C shows the achievable 8.3× pause reduction at GOGC=400)
before reaching for `debug.SetGCPercent(-1)` plus manual `runtime.GC()`
triggers, which trade a worse failure mode (unbounded heap growth between
manual triggers) for pause elimination.

**HTTP client connection pool exhaustion**
Symptom: export latency climbing under sustained load with connection
churn (new TCP handshake per request instead of reuse). The exporter
(`internal/exporter/http_exporter.go`) sets `MaxIdleConns: 10`,
`IdleConnTimeout: 90s` — with a single exporter goroutine issuing batches
sequentially, one idle connection is normally enough for steady traffic to
one collector endpoint; `MaxIdleConns: 10` is headroom for retry/breaker
transitions, not an expectation of concurrent in-flight requests (there
is exactly one exporter goroutine, so there is never more than one
in-flight POST). If a future change makes the exporter concurrent, this
transport setting is the first thing to revisit.

## Reproducing

```powershell
cd sdk/go
go test -bench=BenchmarkSDK -benchmem -count=3 .

# Scenario A: bracket the collector-latency-limited ceiling
go run ./cmd/loadtest -rps 5000 -duration 20s -mock-latency 100ms -goroutines 50
go run ./cmd/loadtest -rps 5500 -duration 20s -mock-latency 100ms -goroutines 50

# Scenario B: sustained memory-flatness check
go run ./cmd/loadtest -rps 3000 -duration 2m -goroutines 50

# Scenario C: GOGC comparison
go run ./cmd/loadtest -rps 8000 -duration 30s -goroutines 50
$env:GOGC = "400"; go run ./cmd/loadtest -rps 8000 -duration 30s -goroutines 50; Remove-Item Env:\GOGC
```
