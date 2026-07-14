# OpenTrace

> A production-grade, self-hosted observability platform for engineering teams who demand full control over their telemetry data.

[![Go Version](https://img.shields.io/badge/go-1.22+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](LICENSE)
[![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-semantic%20conventions-f5a623.svg)](https://opentelemetry.io)

**Status:** Active development — Day 30 of 35, Phase 2 (SDK Development). Phase 0 (architecture) and Phase 1 (Log Ingestion MVP) are complete and merged to `main`; the Go SDK and backend integration are in progress.

---

## Vision

Modern observability tools like Datadog and Honeycomb solve real problems — but they do so at the cost of data sovereignty, unpredictable pricing, and vendor lock-in. OpenTrace is the alternative: a fully self-hosted observability platform that delivers the same core capabilities (structured logs, dimensional metrics, distributed traces) without sending your telemetry to a third party.

OpenTrace is built for teams that need:
- **Data residency guarantees** — your telemetry never leaves your infrastructure
- **Predictable costs** — storage and compute scale with your infrastructure, not your event volume
- **Full query flexibility** — raw SQL access to ClickHouse and PostgreSQL, not a locked-down query language
- **OpenTelemetry alignment** — data model targets OTEL Semantic Conventions v1.24+; OTLP wire-level compatibility is planned but not yet validated end-to-end

---

## Architectural Tenets

| Tenet | Implementation |
|-------|---------------|
| **High write throughput** | Async ingestion via Redpanda decouples SDK clients from storage writes; ClickHouse's columnar engine is designed for high-throughput bulk inserts (bulk-insert throughput not yet benchmarked at production scale) |
| **Low query latency** | Composite indexes on (service, severity, timestamp) enable index-only scans; keyset pagination avoids COUNT(*) over large result sets. Targets sub-100ms on typical queries; measured p95 under 200 concurrent k6 VUs on constrained dev hardware was ~5s — see [performance-baseline.md](docs/internal/runbooks/performance-baseline.md) for the full breakdown and caveats |
| **Operational simplicity** | Single `docker compose up` starts the full stack; Kubernetes manifests and Helm charts for production deployment |
| **Schema-forward design** | OpenTelemetry Semantic Conventions v1.24+ compliance for all wire formats; versioned Protobuf definitions for stable API contracts |
| **Zero SDK overhead** | Go SDK hot path measured at **0 heap allocations per log call**, enforced by a build-breaking allocation-budget test; ~5–7M calls/s single-goroutine throughput. Background goroutine handles all I/O — see [performance-baseline.md](docs/internal/runbooks/performance-baseline.md) |

---

## Technology Choices

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Backend services | Go 1.22+ | Predictable latency, excellent concurrency primitives, small Docker images |
| Primary storage | ClickHouse 24.x | Columnar MergeTree engine — columnar stores are commonly cited at 10–100× better compression/query speed vs row stores for time-series data (industry benchmark; not yet measured on OpenTrace's own dataset) |
| Secondary storage | PostgreSQL 16 + TimescaleDB | JSONB flexibility, rich indexing, excellent operational tooling |
| Message queue | Redpanda | Kafka-compatible; single binary, no JVM. Redpanda's own benchmarks cite lower latency than Kafka for small batches — not independently verified by this project |
| Frontend | React 19 + TypeScript + Vite | Type-safe, fast HMR, TanStack Router for type-safe navigation |
| Monorepo tooling | Go Workspaces (go.work) | Independent SDK versioning while sharing internal packages |

---

## System Requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| Go | 1.22+ | 1.22+ |
| Docker | 24.0+ | 25.0+ |
| Docker Compose | 2.20+ | 2.24+ |
| RAM (local dev) | 8 GB | 16 GB |
| Disk (local dev) | 10 GB free | 20 GB free |
| CPU | 4 cores | 8 cores |

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│  SDK Instrumentation Layer                                               │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐      │
│  │  Go SDK           │  │  (planned) JS SDK │  │  OTEL Collector  │      │
│  │  opentrace-go     │  │  opentrace-js     │  │  (planned)       │      │
│  └────────┬─────────┘  └────────┬─────────┘  └────────┬─────────┘      │
│           │ HTTP/gRPC            │ HTTP/gRPC            │ HTTP/gRPC       │
└───────────┼──────────────────────┼──────────────────────┼────────────────┘
            │                      │                      │
            ▼                      ▼                      ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Edge Ingress                                                            │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  Load Balancer / Nginx (HTTP/2, TLS termination)                 │   │
│  └──────────────────────────────┬──────────────────────────────────┘   │
└─────────────────────────────────┼───────────────────────────────────────┘
                                  │ HTTP/2
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Collector Service  [stateless, horizontally scalable]                   │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  POST /api/v1/logs  │  POST /api/v1/metrics  │  POST /api/v1/traces│  │
│  │  Validate → Enqueue → Acknowledge (async, 202 Accepted)          │   │
│  └──────────────────────────────┬──────────────────────────────────┘   │
└─────────────────────────────────┼───────────────────────────────────────┘
                                  │ Kafka protocol
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Redpanda Message Broker  [stateful, replicated]                         │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────────┐      │
│  │  logs topic   │  │ metrics topic │  │  traces topic             │      │
│  │  (partitioned)│  │ (partitioned) │  │  (partitioned)            │      │
│  └──────┬───────┘  └──────┬───────┘  └──────────┬───────────────┘      │
└─────────┼─────────────────┼────────────────────────┼────────────────────┘
          │                 │                        │
          ▼                 ▼                        ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Ingestion Workers  [stateless, horizontally scalable]                   │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  Consume → Deserialize → Bulk Insert (pgx CopyFrom / CH batch)  │   │
│  └────────────────────┬──────────────────────────┬─────────────────┘   │
└───────────────────────┼──────────────────────────┼─────────────────────┘
                        │                          │
            ┌───────────▼───────────┐  ┌───────────▼───────────┐
            │  ClickHouse Cluster   │  │  PostgreSQL 16 +       │
            │  [stateful]           │  │  TimescaleDB           │
            │  Logs, Metrics,       │  │  [stateful]            │
            │  Traces (columnar)    │  │  Metadata, JSONB attrs │
            └───────────┬───────────┘  └───────────┬───────────┘
                        │                          │
                        └──────────┬───────────────┘
                                   │
                                   ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Query API  [stateless, horizontally scalable]                           │
│  ┌─────────────────────────────────────────────────────────────────┐   │
│  │  GET /api/v1/logs  │  GET /api/v1/metrics/query  │  GET /traces  │   │
│  │  Dynamic filter → Index scan → Keyset pagination → JSON response │   │
│  └──────────────────────────────┬──────────────────────────────────┘   │
└─────────────────────────────────┼───────────────────────────────────────┘
                                  │ HTTP/JSON
                                  ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  React UI  [stateless]                                                   │
│  Nginx serves SPA, proxies /api/* to Query API                          │
│  TanStack Router + React Query + Virtualized log/trace tables            │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## Quick Start (Local Development)

**Prerequisites:** Docker 24+, Docker Compose 2.20+, Go 1.22+, Node 20+

```bash
# 1. Clone the repository
git clone https://github.com/mirkodibr/OpenTrace.git
cd OpenTrace

# 2. Copy and configure environment
cp .env.example .env
# Edit .env to set any required values (defaults work for local dev)

# 3. Start the full stack
make up

# 4. Verify all services are healthy
make health

# 5. Open the dashboard
open http://localhost:3000

# 6. Send sample telemetry
make seed
```

**Service endpoints (local):**
| Service | URL |
|---------|-----|
| OpenTrace UI | http://localhost:3000 |
| Collector API | http://localhost:8080 |
| Query API | http://localhost:8081 |
| Redpanda Console | http://localhost:8082 |
| ClickHouse HTTP | http://localhost:8123 |

---

## Repository Structure

```
opentrace/
├── cmd/                        # Binary entry points (one dir per service)
│   ├── collector-service/      # Telemetry ingest binary
│   └── query-api/              # Dashboard query binary
├── internal/                   # Private service logic (not importable externally)
│   ├── collector-service/      # Collector handler, config, middleware
│   └── query-api/              # Query handler, config, middleware
├── pkg/                        # Shared, importable libraries
│   └── schema/                 # Canonical telemetry data types
├── api/                        # Contract definitions (versioned)
│   └── v1/
│       ├── proto/              # Protobuf service definitions
│       └── openapi/            # OpenAPI 3.1 specs
├── sdk/
│   └── go/                     # Go client SDK (separate Go module)
├── web/                        # React frontend (separate npm workspace)
├── infra/
│   ├── docker/                 # Dockerfiles for each service
│   ├── postgres/               # SQL migration scripts
│   ├── clickhouse/             # ClickHouse DDL and migrations
│   └── k8s/                    # Kubernetes manifests and Helm charts
├── docs/
│   ├── internal/               # ADRs, runbooks, design docs
│   └── public/                 # User-facing guides
├── scripts/                    # CI/CD automation, developer tooling
├── tools/                      # Pinned Go tool dependencies
├── docker-compose.yml
├── go.work                     # Go Workspace definition
├── Makefile
└── .env.example
```

---

## Roadmap

- **Phase 2 (in progress, Day 30/35)** — Go SDK, backend integration, performance hardening, load testing
- **JS/TS SDK** — planned, not started
- **OTLP wire compatibility** — planned; current wire format is OpenTrace-native (see [ADR-006](docs/internal/adr/006-sdk-wire-format.md))
- **Phase 3** — auth layer, production deployment hardening (see [prompt library](opentrace_prompt_library.md) for the full day-by-day plan)

---

## Documentation

- [Architecture Decision Records](docs/internal/adr/) — Design decisions and rationale
- [API Reference](api/v1/openapi/) — OpenAPI 3.1 specifications
- [SDK source](sdk/go/) — Go SDK package (dedicated integration guide not yet written)
- [Runbooks](docs/internal/runbooks/) — Operational procedures, including the [measured performance baseline](docs/internal/runbooks/performance-baseline.md)

> Contributing guide is not yet written — `docs/public/` doesn't exist yet. Open an issue if you'd like to contribute before it lands.

---

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
