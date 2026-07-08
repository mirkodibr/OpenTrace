# E2E Validation Playbook

Run this playbook after every deployment (and after any change to the SDK
serialiser, collector validation, or PostgreSQL schema) to prove the full
pipeline works: **SDK → collector-service → PostgreSQL → query-api → UI**.

## Quick smoke (2 minutes)

```powershell
# 1. Stack up (dev override publishes ports; backend net is internal-only)
docker compose -f docker-compose.yml -f docker-compose.dev.yml up -d `
    postgres clickhouse redpanda collector-service query-api

# 2. Health
curl http://localhost:8080/healthz     # collector: status ok, database ok, broker ok
curl http://localhost:8081/healthz     # query-api: status ok

# 3. Automated end-to-end proof (exit code 0 = pass)
go run ./cmd/e2e-verify                     # gzip path (default)
go run ./cmd/e2e-verify -no-compression    # uncompressed regression
```

`e2e-verify` sends uniquely tagged events through the real SDK, flushes,
polls the query API, and asserts count + attribute integrity + timing.

## Latency measurement points (per hop)

| Hop | Path | How to measure | Target | Measured 2026-07-08 |
|-----|------|----------------|--------|---------------------|
| 1 | `logger.Info()` → buffer | `go test -bench=BenchmarkLoggerInfo -benchmem ./sdk/go` | < 1 µs, 0 allocs | 130–500 ns, 0 allocs |
| 2 | batch trigger → HTTP first byte | SDK `WithDebug(true)` — exporter logs `exported N events (…B) in …` | < 5 ms | sub-ms serialise + gzip |
| 3 | HTTP round-trip to 202 | same debug line (includes full POST) | < 50 ms local | 3–50 ms |
| 4 | collector receive → PG commit | collector log `logs ingested … duration_ms` | < 100 ms / 100-event batch | 3–52 ms |
| 5 | PG commit → query visibility | `e2e-verify` poll timing | < 10 ms (read-your-writes) | first poll (≤ 100 ms incl. poll interval) |
| — | **Total: SDK call → queryable** | `e2e-verify`: flush + visibility | **< 2 s** | **~1.0 s** (0.9 s batch flush + ~0.1 s) |

## Operational debugging queries

Run inside the database: `docker exec -it opentrace-postgres-1 psql -U opentrace -d opentrace`

```sql
-- (a) Is recent ingest working? Expect count > 0 and lag < 30 seconds
--     while traffic is flowing.
SELECT COUNT(*) AS recent, MAX(received_at) AS newest,
       NOW() - MAX(received_at) AS lag
FROM logs
WHERE received_at > NOW() - INTERVAL '5 minutes';

-- (b) Data gaps: one row per minute over the last hour; empty minutes are
--     ingest outages (or simply no traffic).
SELECT t.minute, COUNT(l.id) AS event_count
FROM generate_series(date_trunc('minute', NOW() - INTERVAL '1 hour'),
                     date_trunc('minute', NOW()),
                     INTERVAL '1 minute') AS t(minute)
LEFT JOIN logs l ON date_trunc('minute', l.timestamp) = t.minute
GROUP BY t.minute ORDER BY t.minute;

-- (c) Schema violations that slipped through validation. Expect 0.
SELECT COUNT(*) FROM logs WHERE service_name IS NULL OR body IS NULL;

-- (d) Write throughput per second, last 5 minutes.
SELECT date_trunc('second', received_at) AS second,
       COUNT(*) AS events_per_second
FROM logs
WHERE received_at > NOW() - INTERVAL '5 minutes'
GROUP BY 1 ORDER BY 1;

-- (e) Partition coverage: the NEXT few days must already have partitions,
--     or inserts will fail with "no partition of relation logs found".
SELECT inhrelid::regclass::text AS partition
FROM pg_inherits WHERE inhparent = 'logs'::regclass
ORDER BY 1 DESC LIMIT 5;
```

## Symptom decision trees

### SDK reports events sent, nothing in PostgreSQL

1. Collector logs (`docker logs opentrace-collector-service-1`):
   4xx → payload validation failure; compare against ADR-006 and run the
   golden contract test (`go test -run TestSDKWireFormatContract ./internal/collector-service/handler`).
2. Collector logs: 5xx `bulk insert failed`?
   - `no partition of relation "logs" found for row` → partition window
     exhausted; run query (e) and `SELECT create_log_partition(CURRENT_DATE);`
     — then fix the scheduled pre-creation (init script / cron).
   - connection pool errors → check `pg_stat_activity` count vs `max_connections`.
3. Compression alignment: `curl -s -o NUL -w "%{http_code}" -H "Content-Encoding: gzip" --data-binary @batch.json.gz http://localhost:8080/api/v1/logs`
   — a 400 means the decompress middleware is missing/misordered.
4. Rate limiting / 429: check `X-RateLimit-Remaining` on responses.
5. From the host, can you even reach it? The backend Docker network is
   `internal: true`; the dev override must attach published services to the
   frontend network (see docker-compose.dev.yml). `docker port <container>`
   showing no lines while `HostConfig.PortBindings` has entries = this bug.

### Events in PostgreSQL but not visible in the UI / query API

1. Same database? Compare `QUERY_API_DATABASE_URL` vs `COLLECTOR_DATABASE_URL`.
2. Run query (b) — are the events inside the requested time range?
   The UI defaults to a recent window; events with skewed timestamps
   (host clock, forgotten UTC conversion) fall outside it.
3. Severity filter excluding the events? Query without filters first.
4. Cursor pagination: an old cursor pins the query to a page that no longer
   matches; retry without `cursor`.
5. Keyword search only matches the **body** (full-text), not attributes —
   searching for an attribute value returns nothing by design.

### High DroppedCount() in the SDK

1. Circuit breaker open? Enable `WithDebug(true)` — look for `retry N after …`
   followed by silence (breaker fails fast without log lines once open).
2. Collector p99 vs SDK `HTTPTimeout` (8s default): if the collector is
   slower, every batch times out client-side. Check hop-4 duration_ms.
3. Rate: BatchSize/BatchInterval too small for the event rate → flushCh
   saturates. Increase `WithBatchSize` or shorten `WithBatchInterval`.
4. Collector CPU-throttled in Docker (`docker stats`) → raise limits.

## UI check (manual)

Open http://localhost:3000, set the time range to the last 15 minutes and
filter `service_name = e2e-verify` — the verification events must be visible
with their attributes in the expansion panel.
