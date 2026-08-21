# Querying Nested JSONB Attributes

The `logs.log_attributes` column is `JSONB` and now carries arbitrarily
nested structured metadata (SDK `Object`/`Map`/`StringSlice` fields, Day 31).
This runbook covers querying nested values without regressing to full table
scans.

## Schema recap

`log_attributes JSONB` already has a GIN index (see
[ADR-004](../adr/004-indexing-strategy.md)):

```sql
CREATE INDEX idx_logs_attributes_gin ON logs USING GIN (log_attributes jsonb_path_ops);
```

`jsonb_path_ops` (rather than the default `jsonb_ops`) indexes only the
`@>` containment operator, producing an index roughly a third the size with
faster containment lookups — which is exactly the query shape below. The
trade-off: it cannot serve key-existence (`?`) queries; use the default
opclass only if you need those.

## (a) Query by nested value — containment

Find events where `user.role = 'admin'`:

```sql
SELECT id, timestamp, service_name, body
FROM logs
WHERE timestamp >= now() - interval '1 hour'
  AND log_attributes @> '{"user": {"role": "admin"}}'::jsonb;
```

`@>` (contains) is the index-friendly operator — it matches the GIN
`jsonb_path_ops` index. Verify the plan uses it:

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT ... WHERE log_attributes @> '{"user":{"role":"admin"}}'::jsonb;
```

Expect a `Bitmap Index Scan on idx_logs_attributes_gin` feeding a
`Bitmap Heap Scan`, not a `Seq Scan`. Always keep the partition-pruning
time-range predicate: it restricts the scan to the relevant daily
partitions before the GIN lookup.

**Avoid** the path-extraction form for filtering:

```sql
-- Anti-pattern: ->> cannot use the GIN index, forces a scan+filter.
WHERE log_attributes -> 'user' ->> 'role' = 'admin'
```

## (b) Generated column for a hot nested field

When one nested field is queried constantly (e.g. `user_id`), promote it to
a generated column with its own B-Tree:

```sql
ALTER TABLE logs
  ADD COLUMN user_id TEXT
  GENERATED ALWAYS AS (log_attributes ->> 'user_id') STORED;

CREATE INDEX idx_logs_user_id ON logs (user_id) WHERE user_id IS NOT NULL;
```

Trade-off: sub-millisecond equality/range lookups on `user_id`, but the
column is fixed at schema time — it cannot cover arbitrary user-defined
attributes, and each added generated column is extra write cost. Reserve it
for the handful of fields that dominate dashboard queries.

Nested extraction works too (Postgres 12+):

```sql
ADD COLUMN user_role TEXT
  GENERATED ALWAYS AS (log_attributes #>> '{user,role}') STORED;
```

## (c) Partial expression index for a common combination

For a frequent `service_name` + attribute pair, a partial expression index
beats a broad GIN scan and stays small:

```sql
CREATE INDEX idx_logs_service_userid
  ON logs (service_name, (log_attributes ->> 'user_id'))
  WHERE log_attributes ? 'user_id';
```

Prefer this over GIN when: the attribute is present on a minority of rows
(the partial `WHERE` keeps the index tiny) and queries always pair it with
`service_name`. Prefer GIN when the filter shape is open-ended (many
different attribute keys).

## Migration & rollback

- **No DDL is required** to start writing nested attributes — `JSONB`
  already stores nesting; the SDK change is additive and old SDK versions
  keep working (flat attributes are a subset of nested).
- The generated column in (b) is an **additive, non-blocking** migration
  (`ALTER TABLE ... ADD COLUMN ... GENERATED`), applied per environment when
  a field proves hot. Ship it as its own numbered migration.
- **Rollback**: `DROP COLUMN user_id;` — no data loss, since the source of
  truth remains `log_attributes`.
- **Feature flag**: nested-attribute support needs no coordinated rollout —
  producers and consumers are independent. Adopters upgrade the SDK at their
  own pace.

## Depth & sanitisation note

The collector caps user attribute nesting at 5 levels and flattens anything
deeper to a single top-level dot-notation key (e.g. `a.b.c.d.e.f`), so query
paths never need to descend more than 5 objects. See
`internal/collector-service/handler/sanitize.go` and its tests.
