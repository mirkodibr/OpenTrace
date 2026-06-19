# Query Performance Run-Book

Reference guide for diagnosing and resolving slow query conditions on the OpenTrace `logs` table.

---

## Index Map

| Index | Type | Serves |
|-------|------|--------|
| `idx_logs_service_severity_ts` | B-Tree `(service_name, severity, timestamp DESC)` | Dashboard filter: service + severity + time range |
| `idx_logs_ts_id` | B-Tree `(timestamp DESC, id DESC)` | Keyset pagination anchor; time-range-only queries |
| `idx_logs_log_attributes` | GIN jsonb_path_ops | `log_attributes @> '{"key":"val"}'` containment |
| `idx_logs_resource_attributes` | GIN jsonb_path_ops | `resource_attributes @> '{"host.name":"x"}'` |
| `idx_logs_body_fts` | GIN on `body_tsv` | `body_tsv @@ plainto_tsquery(...)` |
| `idx_logs_received_at_brin` | BRIN | Recent-data queries by ingest time |

**Column order rationale for `idx_logs_service_severity_ts`:**  
service_name is listed first because it has the highest selectivity among the three columns (tens of distinct values vs five for severity). Filtering on service_name alone eliminates the most rows per index page read. Appending severity then timestamp narrows further without additional random I/O.

---

## Reading EXPLAIN ANALYZE

Run the primary dashboard query:

```sql
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT id, timestamp, service_name, severity, body, log_attributes
FROM   logs
WHERE  service_name = 'payment-service'
  AND  severity     = 'error'
  AND  timestamp BETWEEN '2024-01-01T00:00:00Z' AND '2024-01-01T01:00:00Z'
ORDER  BY timestamp ASC, id ASC
LIMIT  100;
```

**What to look for:**

| Node type | Meaning | Action if unexpected |
|-----------|---------|----------------------|
| `Index Scan using idx_logs_service_severity_ts` | Optimal path | None |
| `Bitmap Heap Scan` | Index exists but planner chose bitmap mode — usually acceptable | Check row estimate accuracy |
| `Seq Scan` | No usable index — will timeout on large partitions | Add appropriate index or check column order |
| `Buffers: shared hit=N` | N 8 KB pages read from shared_buffers (fast) | N > 10 000 indicates missing cache warmup |
| `Buffers: shared read=N` | N pages read from disk | High read count → add index or increase shared_buffers |
| `Rows Removed by Filter: N` | Rows the index fetched but the WHERE clause rejected | High ratio → index selectivity is low; consider a partial index |

**Before indexing vs after (annotated plan):**

```
-- BEFORE (no composite index) --
Seq Scan on logs_20240101  (cost=0.00..450000.00 rows=523 width=214)
  Filter: ((service_name = 'payment-service') AND (severity = 'error') AND ...)
  Rows Removed by Filter: 4999477   ← scanning 5M rows to find 523

-- AFTER (idx_logs_service_severity_ts) --
Index Scan using idx_logs_service_severity_ts on logs_20240101
  (cost=0.43..142.32 rows=523 width=214)
  Index Cond: ((service_name = 'payment-service') AND (severity = 'error')
               AND (timestamp >= '...') AND (timestamp <= '...'))
              ← 523 index lookups; 0 rows removed by filter
```

---

## Common Pitfalls

### 1. Timezone conversion invalidates the index

```sql
-- BAD: AT TIME ZONE forces a function call on every row — index not used
WHERE timestamp AT TIME ZONE 'America/New_York' > '2024-01-01'

-- GOOD: store and query in UTC; convert in the application layer
WHERE timestamp > '2024-01-01T05:00:00Z'
```

### 2. NULL handling in composite indexes

```sql
-- trace_id is nullable; this query cannot use a regular index:
WHERE trace_id IS NULL AND service_name = 'auth-service'

-- Fix: partial index for the null case
CREATE INDEX CONCURRENTLY idx_logs_no_trace
    ON logs (service_name, timestamp)
    WHERE trace_id IS NULL;
```

### 3. Leading column missing from composite index

```sql
-- idx_logs_service_severity_ts has (service_name, severity, timestamp)
-- This query skips service_name → Bitmap Heap Scan instead of Index Scan:
WHERE severity = 'error' AND timestamp > '...'

-- Fix: add idx_logs_severity_ts for severity-only queries if needed
CREATE INDEX CONCURRENTLY idx_logs_severity_ts
    ON logs (severity, timestamp DESC);
```

### 4. JSONB attribute query without GIN

```sql
-- BAD (no index path): jsonb operator -> returns text
WHERE log_attributes->>'user_id' = 'u_123'

-- GOOD: @> containment uses the GIN index
WHERE log_attributes @> '{"user_id": "u_123"}'

-- Or add a generated column for the highest-frequency JSONB key:
ALTER TABLE logs ADD COLUMN user_id_attr TEXT
    GENERATED ALWAYS AS (log_attributes->>'user_id') STORED;
CREATE INDEX CONCURRENTLY idx_logs_user_id ON logs (user_id_attr)
    WHERE user_id_attr IS NOT NULL;
```

### 5. Stale planner statistics

```sql
-- Symptom: planner estimates 1 row, actual = 500 000
-- Fix: manual ANALYZE after a large bulk load
ANALYZE logs;

-- Or increase the stats target for high-cardinality columns:
ALTER TABLE logs ALTER COLUMN service_name SET STATISTICS 500;
ANALYZE logs;
```

---

## Daily Health Check Queries

```sql
-- 1. Sequential scans on logs partitions in the last 24 hours
SELECT relname, seq_scan, seq_tup_read, idx_scan
FROM   pg_stat_user_tables
WHERE  relname LIKE 'logs_%'
  AND  seq_scan > 0
ORDER  BY seq_scan DESC;

-- 2. Indexes with zero scans (bloat candidates)
SELECT indexrelname, idx_scan, pg_size_pretty(pg_relation_size(indexrelid))
FROM   pg_stat_user_indexes
WHERE  relname LIKE 'logs%'
  AND  idx_scan = 0
ORDER  BY pg_relation_size(indexrelid) DESC;

-- 3. Top 10 slowest queries (requires pg_stat_statements)
SELECT query, calls, total_exec_time/calls AS avg_ms,
       rows/calls AS avg_rows
FROM   pg_stat_statements
WHERE  query ILIKE '%logs%'
ORDER  BY avg_ms DESC
LIMIT  10;

-- 4. Tables with high autovacuum lag
SELECT relname, n_dead_tup, n_live_tup,
       round(n_dead_tup::numeric / nullif(n_live_tup,0) * 100, 1) AS dead_pct,
       last_autovacuum
FROM   pg_stat_user_tables
WHERE  relname LIKE 'logs%'
ORDER  BY dead_pct DESC NULLS LAST;

-- 5. Partition sizes for the last 14 days
SELECT c.relname AS partition,
       pg_size_pretty(pg_relation_size(c.oid)) AS size,
       s.n_live_tup AS rows
FROM   pg_class c
JOIN   pg_inherits i ON i.inhrelid = c.oid
JOIN   pg_class p    ON p.oid = i.inhparent
LEFT   JOIN pg_stat_user_tables s ON s.relname = c.relname
WHERE  p.relname = 'logs'
ORDER  BY c.relname DESC
LIMIT  14;
```

---

## Maintenance Procedures

```sql
-- REINDEX without downtime (PostgreSQL 12+)
REINDEX INDEX CONCURRENTLY idx_logs_service_severity_ts;

-- Measure index bloat (requires pgstattuple extension)
SELECT pg_size_pretty(pg_relation_size('idx_logs_service_severity_ts')) AS index_size,
       (pgstattuple('idx_logs_service_severity_ts')).dead_leaf_percent AS bloat_pct;
-- Trigger reindex when bloat_pct > 30

-- Safe index removal workflow:
-- 1. Document the index and its last scan date in this run-book
-- 2. DROP INDEX CONCURRENTLY idx_name;
-- 3. Run EXPLAIN ANALYZE on affected queries to verify no regression
```
