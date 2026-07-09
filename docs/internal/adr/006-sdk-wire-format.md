# ADR-006: SDK ↔ Collector Wire-Format Alignment

**Status:** Accepted
**Date:** Phase 2, Day 29

## Context

The Go SDK (Day 25 exporter) and the collector-service ingest handler (Day 8)
were built against the same conceptual schema but from opposite ends. This ADR
pins the exact field-by-field mapping, decides where field flattening happens,
and records the shared-configuration invariants. The mapping is enforced by an
executable golden test:
`internal/collector-service/handler/sdk_contract_test.go` runs the real SDK
against a capture server and decodes the payload into
`schema.IngestLogsRequest` with `DisallowUnknownFields`, then runs the
collector's own validation over it.

## Field mapping

| SDK source | Wire field (`pkg/schema/log.go`) | Transform |
|---|---|---|
| `LogEvent.Timestamp` (time.Time) | `events[].timestamp` | RFC3339Nano, UTC |
| resource `ServiceName` | `events[].service_name` | verbatim |
| `LogEvent.Level` (Level) | `events[].severity` | `Level.String()` → lowercase `debug\|info\|warn\|error\|fatal` |
| `LogEvent.Message` | `events[].body` | verbatim (collector caps at 32,768 chars) |
| Field `trace_id` (from `WithContext`) | `events[].trace_id` | lifted out of attributes to top level |
| Field `span_id` (from `WithContext`) | `events[].span_id` | lifted out of attributes to top level |
| resource struct | `events[].resource_attributes` | OTel keys: `service.name`, `service.version`, `deployment.environment`, `host.name`, `process.pid` |
| remaining `Fields[]` | `events[].log_attributes` | flattened map, see value table |

Field value serialisation:

| Field constructor | JSON type in `log_attributes` |
|---|---|
| `String`, `Err` | string (Err uses `err.Error()`) |
| `Int`, `Int64` | number (integer) |
| `Float64` | number |
| `Bool` | boolean |
| `Duration` | number — **float64 milliseconds** (matches the platform-wide `duration_ms` convention) |
| `Any` | JSON-marshalled as-is; unmarshalable values degrade to their `fmt` string (never poison the batch) |

**Key collisions:** last-wins. Child-logger fields are appended after parent
fields, so the most specific writer wins deterministically.

## Decision: flattening happens in the SDK (D4)

The Day 29 prompt suggested a collector-side `flattenFields` translating an
SDK-specific field array. We deviate: the SDK serialiser emits the canonical
schema directly.

1. **The collector stays SDK-agnostic.** Any client that speaks the documented
   schema (future Python/JS SDKs, curl, vector.dev) ingests identically; the
   collector never needs to know SDK internals.
2. **No double transformation on the hot ingest path.** A collector-side
   flatten would deserialise an SDK envelope and re-shape it per event at
   50k events/s. Emitting the final shape once, in the SDK's background
   goroutine, is strictly cheaper.
3. **Sanitisation stays server-side** (Day 31): limits on depth, key count,
   and key charset are trust-boundary concerns and cannot be delegated to
   clients.

## Configuration consistency invariants

| Invariant | SDK side | Collector side | Status |
|---|---|---|---|
| Batch size ≤ server max | `BatchSize` default 500, max 5000 | `MaxBatchSize` 1000 | ✅ default OK; values > 1000 are rejected server-side — documented on `WithBatchSize` |
| Payload ≤ server max | `MaxBatchBytes` 1 MB default | 5 MB `MaxBytesReader` (decompressed) | ✅ 5× headroom |
| Client times out first | `HTTPTimeout` **8s** (changed from 10s this ADR) | `WriteTimeout` 10s | ✅ fixed timeout inversion |
| Compression alignment | gzip when payload > 1 KB | `middleware.Decompress` before handler | ✅ added this ADR; limit applies to decompressed bytes (gzip-bomb defence) |
| Content-Type | `application/json` | expected | ✅ |
| Severity casing | `Level.String()` lowercase | enum `debug…fatal` | ✅ enforced by golden test |
| `X-SDK-Version` header | sent on every request | logged only (not persisted) | ✅ deviation from the prompt's "pass through to log_attributes": persisting a transport header as a user attribute would collide with user keys; version analytics belong in access logs |

## Consequences

- Any schema change in `pkg/schema/log.go` breaks the golden test — by design;
  the contract is executable.
- Future SDKs must reproduce this mapping; this ADR is their specification.
- `cmd/e2e-verify` validates the full path (SDK → collector → PostgreSQL →
  query-api) against a running stack, including attribute round-trip.
