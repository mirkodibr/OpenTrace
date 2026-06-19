# ADR-004: Definitive Indexing Strategy for the Logs Table

- **Status**: Accepted
- **Date**: 2024-01-13
- **Deciders**: backend platform team

---

## Context

The `logs` table is write-heavy: production telemetry ingests ~50 000 events/sec at peak.
Every index added multiplies write amplification — PostgreSQL must update each index on
every INSERT, and the GIN indexes must update their posting lists on every write to the
indexed column. At the same time, read latency must stay < 200 ms for 95th-percentile
dashboard queries across typical 1-hour windows.

The table is range-partitioned by day (ADR-002). Index sizing and maintenance characteristics
apply **per partition**, not to the entire table.

---

## Decision: Final Index Set

```sql
-- 1. Composite covering index: the primary read path
--    Covers the most common dashboard filter (service + severity + time range).
--    Column order: service_name first (highest selectivity), then severity, then timestamp.
--    Including id enables index-only scans for keyset pagination.
CREATE INDEX CONCURRENTLY idx_logs_service_severity_ts
    ON logs (service_name, severity, timestamp DESC)
    INCLUDE (id);

-- 2. Time + ID index: pagination anchor and time-range-only queries
--    Used when the query has no service_name or severity filter.
--    INCLUDE (id) enables index-only keyset pagination.
CREATE INDEX CONCURRENTLY idx_logs_ts_id
    ON logs (timestamp DESC, id DESC);

-- 3. Full-text search on pre-computed tsvector
--    body_tsv is a GENERATED ALWAYS AS stored column (see ADR-003).
--    GIN is mandatory for tsvector; the alternative (expression index) would
--    recompute to_tsvector on every index update.
CREATE INDEX CONCURRENTLY idx_logs_body_fts
    ON logs USING gin (body_tsv);

-- 4 & 5. JSONB containment: resource and log attribute lookups
--    jsonb_path_ops GIN is 2-3x smaller than the default (jsonb_ops) because
--    it only stores hash paths, not key names. The trade-off is that it cannot
--    answer existence queries (? operator) — acceptable for our use case which
--    is always containment (@>).
CREATE INDEX CONCURRENTLY idx_logs_log_attributes
    ON logs USING gin (log_attributes jsonb_path_ops);

CREATE INDEX CONCURRENTLY idx_logs_resource_attributes
    ON logs USING gin (resource_attributes jsonb_path_ops);

-- 6. BRIN on received_at: operational monitoring
--    BRIN stores min/max per 128-page range block. On an append-only table
--    (inserts are always increasing received_at), every range block has nearly
--    perfect correlation, making BRIN extremely effective.
--    Storage overhead: ~8 KB total per partition regardless of row count.
CREATE INDEX CONCURRENTLY idx_logs_received_at_brin
    ON logs USING brin (received_at) WITH (pages_per_range = 128);
```

---

## Index Type Selection Framework

| Criterion | B-Tree | GIN | BRIN |
|-----------|--------|-----|------|
| Column type | Scalar, comparable | Array, JSONB, tsvector | Scalar, physically ordered |
| Equality / range | Excellent | Poor (use B-Tree instead) | Coarse (block-level) |
| Containment (`@>`) | No | Excellent | No |
| Full-text (`@@`) | No | Excellent | No |
| Write overhead | Low–medium | Medium–high (deferred pending lists) | Near-zero |
| Index size | Medium | Large | Tiny (8 KB typical) |
| Index-only scans | Yes (with INCLUDE) | No | No |

### When to choose each type

**B-Tree**: default choice for scalar columns used in equality, range, or sort operations.
Use `INCLUDE` to add non-key columns that appear in SELECT but not WHERE — this converts
heap fetches to index-only scans when the visibility map is current.

**GIN**: JSONB or tsvector columns with containment or search queries. GIN maintains a
pending list (up to `gin_pending_list_limit`, default 4 MB) that is flushed to the main
index structure in the background, deferring per-row write cost. This is why GIN feels
cheaper on writes than its actual overhead — the cost is batched.

**BRIN**: monotonically increasing columns on large, append-only tables. Suited for
`received_at` or `created_at` where physical storage order matches logical order. Useless
on columns that are inserted in random order (e.g., `trace_id`).

---

## Write vs Read Trade-off Analysis

| Index | Estimated write overhead per INSERT | Expected read benefit |
|-------|-----|---|
| `idx_logs_service_severity_ts` | ~0.3 µs (B-Tree leaf update, usually hits buffer pool) | Eliminates seq-scan on primary dashboard path; reduces 450 000-row scan to 523-row index scan |
| `idx_logs_ts_id` | ~0.2 µs | Pagination and time-range queries without service/severity filter |
| `idx_logs_body_fts` | ~1.5 µs amortised (GIN pending list flush is batched) | Converts tsvector expression evaluation at query time to GIN lookup |
| `idx_logs_log_attributes` | ~1.2 µs amortised | JSONB containment queries go from seq-scan to GIN lookup |
| `idx_logs_resource_attributes` | ~1.2 µs amortised | Same as above for resource attributes |
| `idx_logs_received_at_brin` | < 0.01 µs (near zero; page range update is idempotent) | Eliminates seq-scan on operational time-window queries |
| **Total** | **~4.4 µs per INSERT** | **Predicted: < 200 ms for 95th-percentile dashboard queries** |

At 50 000 events/sec, 4.4 µs of index overhead adds 220 ms total CPU per second —
well within a 4-core collector budget. Disk amplification is the larger concern; the
GIN indexes write ~3× the bytes of the raw data on flush, but deferred pending lists
mean instantaneous write amplification stays closer to 1.5×.

---

## Indexes We Explicitly Rejected

| Candidate | Reason rejected |
|-----------|-----------------|
| `(trace_id)` B-Tree | trace_id is high-cardinality random hex — index would be large and accessed only for point lookups that are uncommon in dashboard paths. Add only if distributed tracing trace-detail view becomes a primary use case. |
| `(body)` B-Tree | Replaced by GIN on `body_tsv`. B-Tree cannot do substring matching efficiently and would be 10× the size. |
| `(severity)` alone | severity has only 5 distinct values — selectivity is too low for an independent index to be useful. Covered by `idx_logs_service_severity_ts` as a non-leading column. |
| `(log_attributes->>'user_id')` expression | user_id is one of hundreds of possible attribute keys. Expression indexes fix one key; GIN covers all keys at lower total cost. Add a generated-column index only for keys proven to be queried at > 100 qps. |
| Hash indexes on `service_name` | Hash indexes support only equality, not range or sort. The composite B-Tree index already handles equality faster due to sorted leaf pages and INCLUDE support. |

---

## Partition-Level Index Creation

New daily partitions must inherit indexes automatically. PostgreSQL propagates indexes from
the parent table to new partitions when they are created via `CREATE TABLE ... PARTITION OF`.
The `CREATE INDEX CONCURRENTLY` commands above are run once on the parent; child partitions
inherit the index definition at creation time.

To verify:
```sql
SELECT indexname, tablename
FROM   pg_indexes
WHERE  tablename LIKE 'logs_2024%'
ORDER  BY tablename, indexname;
```

---

## Maintenance Run-Book

### Monthly: check index bloat

```sql
-- Requires pg_stat_statements and pgstattuple extension
SELECT indexrelname,
       pg_size_pretty(pg_relation_size(indexrelid)) AS index_size,
       (pgstattuple(indexrelid)).dead_leaf_percent AS dead_pct
FROM   pg_stat_user_indexes
WHERE  relname LIKE 'logs%'
ORDER  BY dead_pct DESC;
```

Rebuild any index with `dead_pct > 30`:
```sql
REINDEX INDEX CONCURRENTLY idx_logs_service_severity_ts;
```

### Quarterly: validate index usage

```sql
SELECT indexrelname, idx_scan, idx_tup_read
FROM   pg_stat_user_indexes
WHERE  relname LIKE 'logs%'
ORDER  BY idx_scan ASC;
```

Indexes with `idx_scan = 0` over a 90-day window that are not enforcing a constraint
should be dropped. Follow the safe removal workflow in `docs/internal/runbooks/query-performance.md`.

### On schema change: re-evaluate covering columns

When new query patterns are added (e.g., filtering by `schema_url`), revisit whether the
existing INCLUDE clause on `idx_logs_service_severity_ts` should be extended, or whether
a new partial index is cheaper. The rule: if a query pattern accounts for > 5% of read
traffic and hits more than one partition in a seq-scan, it warrants an index.

---

## Consequences

- Six indexes on the `logs` table add ~4.4 µs write overhead per event — acceptable.
- GIN deferred pending lists require that `gin_pending_list_limit` (default 4 MB per index)
  is not reduced below 1 MB in `postgresql.conf`; doing so increases flush frequency and
  spikes write latency.
- The `INCLUDE` clause on `idx_logs_service_severity_ts` adds `id` to leaf pages; this
  increases index size by ~10% but eliminates heap fetches for paginated responses.
- `BRIN` on `received_at` must be rebuilt once with `VACUUM ANALYZE` when the table is
  first populated; autovacuum maintains it thereafter.
