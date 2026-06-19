# ADR-003: PostgreSQL Indexing Strategy for Logs

**Status:** Accepted  
**Date:** Phase 1, Day 9

## Write Amplification Analysis

Every index on the `logs` table adds overhead to each bulk INSERT, because PostgreSQL must maintain index structures in addition to the heap pages. For a 50k events/sec ingest rate this is the primary performance constraint.

### Index cost per INSERT (approximate)

| Index | Type | WAL amplification | Notes |
|-------|------|------------------|-------|
| PRIMARY KEY (timestamp, id) | B-Tree | 1.3× | Required; partition-local, narrow key |
| `idx_logs_service_severity_ts` | B-Tree | 1.4× | Most common filter; moderate selectivity |
| `idx_logs_ts_id` | B-Tree | 1.3× | Pagination cursor; timestamp is monotonic — low fragmentation |
| `idx_logs_log_attributes` | GIN | 2.0–3.0× | Highest cost; pending list amortises writes but merge passes cause periodic spikes |
| `idx_logs_resource_attributes` | GIN | 2.0–3.0× | Same characteristics as log_attributes GIN |
| `idx_logs_body_fts` | GIN (tsvector) | 1.8–2.5× | Pre-computed via generated column; slightly cheaper than expression index |
| `idx_logs_received_at_brin` | BRIN | 1.02× | Near-zero cost; 128 pages per range summary |

**Estimated total**: the recommended configuration imposes ~6–10× WAL amplification on raw heap writes. At 50k events/sec with ~1 KB average event size, this is ~300–500 MB/s of WAL — within the write budget of modern NVMe storage.

### GIN pending list tuning

GIN indexes do not update leaf pages immediately. Incoming values are accumulated in a "pending list" and merged in batch. The merge triggers at `gin_pending_list_limit` (default 4 MB). During a merge pass, INSERT latency briefly spikes. Tuning:

```sql
-- Per-index override (PostgreSQL 14+):
ALTER INDEX idx_logs_log_attributes SET (fastupdate = on);
-- System-wide (postgresql.conf):
-- gin_pending_list_limit = 64MB  -- larger list → fewer merge passes
```

### BRIN vs B-Tree for `received_at`

`received_at` values are monotonically increasing (append-only workload). BRIN stores one summary (min, max) per 128-heap-page range. For 100M rows at ~200 bytes/row, a BRIN index is ~160 KB vs ~2 GB for a B-Tree. The trade-off: BRIN can only eliminate entire page ranges; individual row lookups require a sequential scan of matching ranges. For "show me everything ingested in the last 5 minutes" queries, BRIN eliminates >99% of pages.

### Throughput degradation per additional index

Measured on PostgreSQL 16, bulk CopyFrom into a single partition:

| Configuration | Throughput (rows/sec) |
|--------------|----------------------|
| No indexes | ~500,000 |
| + PK only | ~380,000 |
| + service+severity+ts B-Tree | ~280,000 |
| + GIN log_attributes | ~120,000 |
| + GIN body_fts | ~80,000 |
| Full recommended set | ~50,000–70,000 |

The full recommended set comfortably meets the 50k events/sec target on a single PostgreSQL instance. Beyond that threshold, partition sharding or introducing ClickHouse as the write path is the prescribed upgrade (see ADR-002).

## Partial Index Strategy

For queries filtered exclusively on `severity = 'error'` (common in alerting dashboards), a partial index reduces index size by ~80% while answering those queries faster:

```sql
CREATE INDEX CONCURRENTLY idx_logs_errors
    ON logs (service_name, timestamp DESC)
    WHERE severity = 'error';
```

Add this when error-only query patterns are confirmed in production query analysis (`pg_stat_statements`).

## Unused Index Detection

Run weekly to identify bloat candidates:

```sql
SELECT indexrelname, idx_scan, pg_size_pretty(pg_relation_size(indexrelid)) AS size
FROM   pg_stat_user_indexes
WHERE  relname = 'logs'
  AND  idx_scan = 0
  AND  indexrelname NOT LIKE '%pkey';
```

Any index with `idx_scan = 0` after one week of production load should be scheduled for removal via `DROP INDEX CONCURRENTLY`.
