-- =============================================================================
-- OpenTrace — ClickHouse Logs Schema
-- Engine: MergeTree (replicated variant for production clusters)
-- =============================================================================

CREATE DATABASE IF NOT EXISTS opentrace;

-- ---------------------------------------------------------------------------
-- Logs table — columnar MergeTree optimised for high-throughput append and
-- fast aggregation over large time windows.
--
-- PRIMARY KEY / ORDER BY rationale:
--   (service_name, severity, toStartOfHour(timestamp), timestamp)
--   service_name  → first: most common filter; groups related data on disk
--   severity      → second: second most common filter in dashboard queries
--   toStartOfHour → coarse time bucketing reduces granule count for range scans
--   timestamp     → fine-grained sort within the bucket
--
-- PARTITION BY:
--   toYYYYMMDD(timestamp) → daily partitions; matches PostgreSQL convention.
--   Enables partition pruning for time-range queries and per-day TTL drops.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS opentrace.logs
(
    -- Identity
    id           String       CODEC(ZSTD(1)),
    trace_id     String       CODEC(ZSTD(1)),
    span_id      String       CODEC(ZSTD(1)),

    -- Timing — Delta encoding then LZ4 exploits monotonic increment pattern
    timestamp    DateTime64(9, 'UTC') CODEC(Delta, LZ4),
    received_at  DateTime64(9, 'UTC') CODEC(Delta, LZ4),

    -- Classification
    service_name LowCardinality(String),     -- LowCardinality: dict-encodes low-cardinality strings
    severity     LowCardinality(String),     -- ~5 distinct values → ~2 bits per row
    severity_text String       CODEC(ZSTD(1)),

    -- Content
    body         String       CODEC(ZSTD(3)), -- Higher compression ratio for human-readable text

    -- Structured metadata — stored as JSON strings; extracted via JSONExtract
    resource_attributes String CODEC(ZSTD(1)),
    log_attributes      String CODEC(ZSTD(1)),

    schema_url   String       CODEC(ZSTD(1))
)
ENGINE = MergeTree
PARTITION BY toYYYYMMDD(timestamp)
ORDER BY (service_name, severity, toStartOfHour(timestamp), timestamp)
TTL toDateTime(timestamp) + INTERVAL 14 DAY DELETE
SETTINGS
    index_granularity = 8192,
    ttl_only_drop_parts = 1;

-- ---------------------------------------------------------------------------
-- Materialized view: fast service + severity aggregate counts.
-- Pre-computes per-hour event counts so the severity filter UI can display
-- counts without scanning the raw logs table.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS opentrace.logs_hourly_counts
(
    service_name LowCardinality(String),
    severity     LowCardinality(String),
    hour         DateTime,
    count        UInt64
)
ENGINE = SummingMergeTree
ORDER BY (service_name, severity, hour)
TTL hour + INTERVAL 30 DAY DELETE;

CREATE MATERIALIZED VIEW IF NOT EXISTS opentrace.logs_hourly_counts_mv
TO opentrace.logs_hourly_counts AS
SELECT
    service_name,
    severity,
    toStartOfHour(timestamp) AS hour,
    count() AS count
FROM opentrace.logs
GROUP BY service_name, severity, hour;
