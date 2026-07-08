-- =============================================================================
-- OpenTrace — PostgreSQL Logs Schema
-- Migration: 001_logs_schema.sql
-- Idempotent: safe to run multiple times.
-- =============================================================================

-- ---------------------------------------------------------------------------
-- Log severity enum — constrains the severity column to known values and
-- enables efficient B-Tree equality scans vs. free-text comparisons.
-- ---------------------------------------------------------------------------
DO $$ BEGIN
    CREATE TYPE log_severity AS ENUM ('debug', 'info', 'warn', 'error', 'fatal');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- ---------------------------------------------------------------------------
-- Main logs table — partitioned by DAY on timestamp.
--
-- Column design rationale:
--   id            BIGINT GENERATED ALWAYS AS IDENTITY
--                 Avoids UUID random write fragmentation in the B-Tree index.
--                 Used as the tiebreaker in keyset pagination cursors.
--
--   timestamp     TIMESTAMPTZ NOT NULL
--                 Stored as UTC. Partition key and primary sort dimension.
--                 Delta+LZ4 compression achieves ~4 bytes/value vs 8.
--
--   received_at   TIMESTAMPTZ NOT NULL DEFAULT now()
--                 Ingest lag measurement: received_at - timestamp = SDK latency.
--                 BRIN index provides near-zero overhead for recent-data scans.
--
--   service_name  TEXT NOT NULL
--                 Low cardinality (tens to hundreds of distinct values).
--                 Leading column in the primary composite index.
--
--   severity      log_severity NOT NULL
--                 Enum column: 4 bytes, fast equality checks, self-documenting.
--
--   body          TEXT NOT NULL
--                 GIN tsvector index supports full-text keyword search.
--
--   resource_attributes / log_attributes   JSONB
--                 OTel resource attributes (host.name, k8s.pod.name, etc.) and
--                 event-level custom key-value pairs. GIN index with
--                 jsonb_path_ops operator class for @> containment queries.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS logs (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY,
    -- TEXT not UUID: OpenTelemetry trace/span IDs are 32-char hex strings
    -- without hyphens; storing as UUID requires conversion overhead and rejects
    -- valid OTel IDs that don't conform to the 8-4-4-4-12 UUID format.
    trace_id            TEXT,
    span_id             TEXT,
    timestamp           TIMESTAMPTZ NOT NULL,
    received_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    service_name        TEXT        NOT NULL,
    severity            log_severity NOT NULL,
    severity_text       TEXT,
    body                TEXT        NOT NULL,
    resource_attributes JSONB,
    log_attributes      JSONB,
    schema_url          TEXT,

    -- Generated column for full-text search — stored on disk so queries use the
    -- GIN index directly without recomputing to_tsvector on every scan.
    body_tsv            TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', body)) STORED,

    -- Partition-local primary key: (timestamp, id) enforces uniqueness within a day.
    PRIMARY KEY (timestamp, id)
) PARTITION BY RANGE (timestamp);

COMMENT ON TABLE logs IS 'Structured log events ingested via the OpenTrace collector-service.';
COMMENT ON COLUMN logs.id IS 'Monotonic identity within the partition. Used as keyset pagination tiebreaker.';
COMMENT ON COLUMN logs.trace_id IS 'OpenTelemetry trace_id for cross-pillar correlation.';
COMMENT ON COLUMN logs.span_id IS 'OpenTelemetry span_id for trace span correlation.';
COMMENT ON COLUMN logs.timestamp IS 'Event creation time as reported by the SDK. UTC only.';
COMMENT ON COLUMN logs.received_at IS 'Time this event was written to the database. Ingest lag = received_at - timestamp.';
COMMENT ON COLUMN logs.service_name IS 'OTel resource attribute: service.name. Low-cardinality filter dimension.';
COMMENT ON COLUMN logs.severity IS 'Normalized log level enum.';
COMMENT ON COLUMN logs.severity_text IS 'Original severity string from the SDK before normalization.';
COMMENT ON COLUMN logs.body IS 'The human-readable log message body.';
COMMENT ON COLUMN logs.resource_attributes IS 'OTel resource attributes: host.name, k8s.pod.name, process.pid, etc.';
COMMENT ON COLUMN logs.log_attributes IS 'Event-level structured attributes: user_id, request_id, http.status_code, etc.';
COMMENT ON COLUMN logs.schema_url IS 'OpenTelemetry schema URL for semantic convention versioning.';

-- ---------------------------------------------------------------------------
-- Indexes — applied to the parent table; PostgreSQL 16 cascades them to all
-- partitions automatically. Use CONCURRENTLY when adding indexes to existing
-- production tables to avoid access-exclusive locks.
-- ---------------------------------------------------------------------------

-- Primary composite index: the most common dashboard query pattern.
-- Column order: service_name first (highest selectivity among our enum set),
-- then severity, then timestamp DESC for reverse-chronological display.
CREATE INDEX IF NOT EXISTS idx_logs_service_severity_ts
    ON logs (service_name, severity, timestamp DESC);

-- Keyset pagination anchor: (timestamp, id) mirrors the cursor structure.
-- Ensures the WHERE (timestamp, id) > (?, ?) pagination clause hits an index.
CREATE INDEX IF NOT EXISTS idx_logs_ts_id
    ON logs (timestamp DESC, id DESC);

-- GIN index on log_attributes for arbitrary JSONB containment queries.
-- jsonb_path_ops is smaller and faster than the default jsonb_ops for @> queries.
CREATE INDEX IF NOT EXISTS idx_logs_log_attributes
    ON logs USING GIN (log_attributes jsonb_path_ops);

-- GIN index on resource_attributes (host.name, k8s.pod.name filters).
CREATE INDEX IF NOT EXISTS idx_logs_resource_attributes
    ON logs USING GIN (resource_attributes jsonb_path_ops);

-- Full-text search via the stored generated column (avoids recomputing tsvector).
CREATE INDEX IF NOT EXISTS idx_logs_body_fts
    ON logs USING GIN (body_tsv);

-- BRIN index on received_at — nearly zero write overhead; useful for
-- "show me everything ingested in the last N minutes" queries which access
-- monotonically increasing blocks.
CREATE INDEX IF NOT EXISTS idx_logs_received_at_brin
    ON logs USING BRIN (received_at);

-- ---------------------------------------------------------------------------
-- Autovacuum tuning for append-heavy workloads.
-- Default autovacuum_vacuum_scale_factor = 0.2 means vacuum triggers at 20%
-- dead tuples. For a 50M row partition that's 10M dead tuples before cleanup.
-- Lowering scale_factor and cost_delay keeps dead tuple count bounded without
-- impacting ingest throughput significantly.
--
-- PostgreSQL does not allow storage parameters on a partitioned (parent)
-- table — "cannot specify storage parameters for a partitioned table" — so
-- these settings are applied per partition at creation time (see the WITH
-- clause in the partition DO block and create_log_partition below).
-- ---------------------------------------------------------------------------

-- Statistics targets — higher values give the query planner better cardinality
-- estimates for high-cardinality columns.
ALTER TABLE logs ALTER COLUMN service_name SET STATISTICS 500;
ALTER TABLE logs ALTER COLUMN severity     SET STATISTICS 200;

-- ---------------------------------------------------------------------------
-- Pre-create 30 daily partitions: today through today+29.
-- The cron job (below) pre-creates tomorrow's partition each night at 23:50.
-- ---------------------------------------------------------------------------
DO $$
DECLARE
    start_date DATE := CURRENT_DATE;
    partition_date DATE;
    partition_name TEXT;
    start_ts TIMESTAMPTZ;
    end_ts TIMESTAMPTZ;
BEGIN
    FOR i IN 0..29 LOOP
        partition_date := start_date + i;
        partition_name := 'logs_' || to_char(partition_date, 'YYYYMMDD');
        start_ts := partition_date::TIMESTAMPTZ;
        end_ts   := (partition_date + 1)::TIMESTAMPTZ;

        IF NOT EXISTS (
            SELECT 1 FROM pg_class c
            JOIN pg_namespace n ON n.oid = c.relnamespace
            WHERE c.relname = partition_name
              AND n.nspname = 'public'
        ) THEN
            EXECUTE format(
                'CREATE TABLE %I PARTITION OF logs FOR VALUES FROM (%L) TO (%L) '
                'WITH (autovacuum_vacuum_scale_factor = 0.01, '
                '      autovacuum_analyze_scale_factor = 0.005, '
                '      autovacuum_vacuum_cost_delay = 2, '
                '      fillfactor = 90)',
                partition_name, start_ts, end_ts
            );
        END IF;
    END LOOP;
END $$;

-- ---------------------------------------------------------------------------
-- Partition management procedures
-- ---------------------------------------------------------------------------

-- create_log_partition: idempotently creates a daily partition for a given date.
-- Called by pg_cron at 23:50 each night to pre-create tomorrow's partition.
CREATE OR REPLACE FUNCTION create_log_partition(p_date DATE)
RETURNS void LANGUAGE plpgsql AS $$
DECLARE
    partition_name TEXT := 'logs_' || to_char(p_date, 'YYYYMMDD');
    start_ts TIMESTAMPTZ := p_date::TIMESTAMPTZ;
    end_ts   TIMESTAMPTZ := (p_date + 1)::TIMESTAMPTZ;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_class c
        JOIN pg_namespace n ON n.oid = c.relnamespace
        WHERE c.relname = partition_name AND n.nspname = 'public'
    ) THEN
        EXECUTE format(
            'CREATE TABLE %I PARTITION OF logs FOR VALUES FROM (%L) TO (%L) '
            'WITH (autovacuum_vacuum_scale_factor = 0.01, '
            '      autovacuum_analyze_scale_factor = 0.005, '
            '      autovacuum_vacuum_cost_delay = 2, '
            '      fillfactor = 90)',
            partition_name, start_ts, end_ts
        );
        RAISE NOTICE 'Created partition: %', partition_name;
    END IF;
END $$;

-- drop_old_log_partitions: drops partitions older than retention_days.
-- Called by pg_cron once daily to enforce the rolling retention window.
CREATE OR REPLACE FUNCTION drop_old_log_partitions(retention_days INT DEFAULT 14)
RETURNS INT LANGUAGE plpgsql AS $$
DECLARE
    r RECORD;
    cutoff TIMESTAMPTZ := now() - (retention_days || ' days')::INTERVAL;
    dropped INT := 0;
BEGIN
    FOR r IN
        SELECT c.relname AS partition_name,
               pg_get_expr(c.relpartbound, c.oid) AS bound_expr
        FROM   pg_class c
        JOIN   pg_inherits i ON i.inhrelid = c.oid
        JOIN   pg_class p ON p.oid = i.inhparent
        WHERE  p.relname = 'logs'
          AND  c.relkind = 'r'
    LOOP
        -- Extract the upper bound timestamp from the partition expression.
        -- Partition names follow logs_YYYYMMDD; parse the date from the name.
        DECLARE
            part_date DATE;
        BEGIN
            part_date := to_date(substring(r.partition_name FROM '\d{8}$'), 'YYYYMMDD');
            IF part_date::TIMESTAMPTZ < cutoff THEN
                EXECUTE format('DROP TABLE IF EXISTS %I', r.partition_name);
                RAISE NOTICE 'Dropped partition: %', r.partition_name;
                dropped := dropped + 1;
            END IF;
        EXCEPTION WHEN others THEN
            RAISE WARNING 'Could not parse date from partition: %', r.partition_name;
        END;
    END LOOP;
    RETURN dropped;
END $$;

-- ---------------------------------------------------------------------------
-- pg_cron schedules (requires pg_cron extension; uncomment when available):
--
-- Pre-create tomorrow's partition at 23:50 each night:
-- SELECT cron.schedule('create-log-partition', '50 23 * * *',
--     $$SELECT create_log_partition(CURRENT_DATE + 1)$$);
--
-- Drop partitions older than 14 days at 01:00 each night:
-- SELECT cron.schedule('drop-old-log-partitions', '0 1 * * *',
--     $$SELECT drop_old_log_partitions(14)$$);
-- ---------------------------------------------------------------------------
