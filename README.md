# OpenTrace

> A production-grade, self-hosted observability platform for engineering teams who demand full control over their telemetry data.

[![Go Version](https://img.shields.io/badge/go-1.22+-blue.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-green.svg)](LICENSE)
[![OpenTelemetry](https://img.shields.io/badge/OpenTelemetry-compatible-f5a623.svg)](https://opentelemetry.io)

---

## Vision

Modern observability tools like Datadog and Honeycomb solve real problems — but they do so at the cost of data sovereignty, unpredictable pricing, and vendor lock-in. OpenTrace is the alternative: a fully self-hosted observability platform that delivers the same core capabilities (structured logs, dimensional metrics, distributed traces) without sending your telemetry to a third party.

OpenTrace is built for teams that need:
- **Data residency guarantees** — your telemetry never leaves your infrastructure
- **Predictable costs** — storage and compute scale with your infrastructure, not your event volume
- **Full query flexibility** — raw SQL access to ClickHouse and PostgreSQL, not a locked-down query language
- **OpenTelemetry compatibility** — wire-compatible with the OTEL ecosystem; existing instrumentation works without changes

---

## Architectural Tenets

| Tenet | Implementation |
|-------|---------------|
| **High write throughput** | Async ingestion via Redpanda decouples SDK clients from storage writes; ClickHouse columnar engine handles 50k+ events/sec bulk inserts |
| **Sub-100ms query latency** | Composite indexes on (service, severity, timestamp) enable index-only scans; keyset pagination avoids COUNT(*) over large result sets |
| **Operational simplicity** | Single `docker compose up` starts the full stack; Kubernetes manifests and Helm charts for production deployment |
| **Schema-forward design** | OpenTelemetry Semantic Conventions v1.24+ compliance for all wire formats; versioned Protobuf definitions for stable API contracts |
| **Zero SDK overhead** | Go SDK designed for 0 heap allocations per log call on the hot path; background goroutine handles all I/O |

---

## Technology Choices

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Backend services | Go 1.22+ | Predictable latency, excellent concurrency primitives, small Docker images |
| Primary storage | ClickHouse 24.x | Columnar MergeTree engine; 10–100× better compression and query speed vs row stores for time-series data |
| Secondary storage | PostgreSQL 16 + TimescaleDB | JSONB flexibility, rich indexing, excellent operational tooling |
| Message queue | Redpanda | Kafka-compatible; single binary, no JVM, 3× lower latency than Kafka for small batches |
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
│  │  Go SDK           │  │  (future) JS SDK  │  │  OTEL Collector  │      │
│  │  opentrace-go     │  │  opentrace-js     │  │  (compatible)    │      │
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
git clone https://github.com/opentrace/opentrace.git
cd opentrace

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

## Documentation

- [Architecture Decision Records](docs/internal/adr/) — Design decisions and rationale
- [API Reference](api/v1/openapi/) — OpenAPI 3.1 specifications
- [SDK Documentation](sdk/go/README.md) — Go SDK integration guide
- [Contributing Guide](docs/public/contributing.md) — How to contribute
- [Runbooks](docs/internal/runbooks/) — Operational procedures

---

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
