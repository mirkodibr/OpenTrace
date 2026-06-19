# ADR-002: Dual Storage Strategy (ClickHouse + PostgreSQL)

**Status:** Accepted  
**Date:** Phase 0

## Context

Two storage engines are in the architecture: ClickHouse (columnar, MergeTree) and PostgreSQL 16 + TimescaleDB. The question is whether to use both or collapse to one.

## Decision

Dual-write strategy:
- **Phase 1 MVP:** PostgreSQL only (simpler operational model for a small team)
- **Scale trigger:** Introduce ClickHouse write path when PostgreSQL bulk insert throughput is saturating (measurable threshold: p99 insert latency > 50ms sustained for 30+ minutes)
- **Long-term:** ClickHouse as primary for time-series aggregation queries; PostgreSQL retained for metadata, JSONB flexibility, and rich indexing on structured attributes

## Rationale

**ClickHouse advantages at scale:**
- Columnar MergeTree engine compresses timestamp + LowCardinality columns 10-100× vs. PostgreSQL heap pages
- GROUP BY aggregations over 30-day windows run in seconds vs. minutes
- Native batch-insert throughput exceeds 500k rows/sec on commodity hardware

**PostgreSQL advantages at MVP stage:**
- Rich indexing on JSONB attributes (GIN) enables arbitrary key-value queries without schema migrations
- Single operational system reduces the on-call surface for a small team
- TimescaleDB hypertables provide time-series partitioning without a separate process
- ACID transactions with pgx CopyFrom provides safe bulk insert semantics

**Why not PostgreSQL only long-term:**
- WAL write amplification from multiple indexes limits raw insert throughput to ~50k rows/sec
- GROUP BY aggregations over large time windows cause sequential scans even with partial indexes
- Autovacuum overhead grows proportionally with table size and update frequency

## Migration Trigger

Introduce ClickHouse when **any** of the following is sustained for >30 minutes:
- PostgreSQL p99 bulk insert latency > 50ms
- PostgreSQL CPU > 80% (likely WAL write amplification)
- PostgreSQL disk write IOPS > 80% of provisioned capacity

## Consequences

- Phase 1 schema is PostgreSQL-only; ClickHouse schema is defined but not wired up
- Query-api must abstract the storage backend behind a repository interface to allow swapping without handler changes
- ClickHouse does not support transactions; duplicate-event handling must be idempotent at the application layer
