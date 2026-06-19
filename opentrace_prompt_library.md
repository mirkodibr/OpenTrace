# OpenTrace — CLI-Ready Prompt Library
> A production-grade, self-contained prompt library for building the OpenTrace observability platform.
> Each prompt is designed to be pasted directly into a CLI AI assistant (e.g., Claude Code, Cursor, Aider) and executed in isolation — no prior context required.

---

## Table of Contents

- [Phase 0 — Repository & Architecture](#phase-0--repository--architecture)
- [Phase 1 — Log Ingestion MVP](#phase-1--log-ingestion-mvp)
- [Phase 2 — SDK Development](#phase-2--sdk-development)

---

## Phase 0 — Repository & Architecture

---

### Day 1 — Monorepo Structure & Architectural Blueprint

**Intent:** Establish the canonical repository layout and system architecture for the entire OpenTrace platform before a single line of application code is written.

```
ROLE: You are a Principal Distributed Systems Engineer with deep expertise in cloud-native 
observability platforms, CNCF ecosystem projects (OpenTelemetry, Jaeger, Prometheus), and 
production Go monorepo design. You have designed systems processing 100,000+ events per second.

PROJECT CONTEXT:
You are designing the greenfield GitHub repository structure and high-level architectural 
blueprint for "OpenTrace" — a production-grade, self-hosted observability platform inspired 
by Datadog and Honeycomb. The platform must handle 50,000+ events per second at steady state 
with sub-100ms end-to-end ingest-to-query latency.

TECH STACK:
- Backend: Go 1.22+ microservices using Go Workspaces (monorepo)
- Frontend: React 19 + TypeScript + Vite
- SDKs: Multi-language, starting with Go
- Infrastructure: Docker, Kubernetes manifests, Helm charts
- Storage: ClickHouse (primary), PostgreSQL 16 + TimescaleDB (secondary)
- Message Queue: Redpanda (Kafka-compatible)

DELIVERABLES — provide all of the following:

1. MONOREPO FOLDER STRUCTURE
   Produce a complete, annotated directory tree following golang-standards/project-layout 
   conventions. The tree must explicitly show:
   - /cmd/<service-name>/main.go entry points for each binary
   - /internal/<service-name>/ for private service logic (handler, server, middleware, config)
   - /pkg/ for shared, importable libraries (e.g., telemetry schema types, validation helpers)
   - /api/ for Protobuf/Avro/OpenAPI contract definitions (versioned: /api/v1/, /api/v2/)
   - /sdk/go/ for the Go client SDK
   - /web/ for the React frontend workspace
   - /infra/ for Docker Compose, Kubernetes manifests, and Helm charts
   - /docs/ split into /docs/internal/ (ADRs, runbooks) and /docs/public/ (user guides)
   - /scripts/ for CI/CD automation and developer tooling
   - /tools/ for pinned Go tool dependencies (go generate targets)
   Annotate every non-obvious directory with a one-line comment explaining its purpose and 
   ownership boundary.

2. STRUCTURAL RATIONALE
   Write a 400–600 word architectural justification covering:
   - Why internal/ vs pkg/ boundary matters for enforcing dependency rules between services
   - Why Protobuf/Avro schemas live in /api/ and not inside each service
   - How Go Workspaces (go.work) enable independent module versioning while sharing code
   - The rationale for co-locating infrastructure code with application code in a monorepo 
     vs. a separate infra repository

3. README.md DRAFT
   Write a production-grade README.md (minimum 600 words) as it would appear on an elite 
   CNCF open-source project. Include:
   - Project vision statement and problem it solves vs. existing tools
   - Architectural tenets (low latency, high write throughput, operational simplicity)
   - Technology choices with one-line justifications
   - System requirements (Go version, Docker version, hardware minimums)
   - Local development quick-start (clone → configure → docker compose up → verify)
   - Link placeholders for Architecture Docs, API Reference, SDK Docs, Contributing Guide

4. ASCII SYSTEM ARCHITECTURE DIAGRAM
   Draw a detailed ASCII art diagram showing the complete data flow:
   SDK Instrumentation → Edge Ingress (Load Balancer) → Collector Service (fan-out) → 
   Redpanda Topic → Ingestion Worker → ClickHouse + PostgreSQL → Query API → React UI
   Include network boundaries, protocol labels (gRPC, HTTP/2, Kafka protocol), and 
   indicate which components are stateless vs. stateful.

QUALITY BAR:
- Output must read as if written by an OpenTelemetry core maintainer.
- Zero boilerplate, placeholder text, or student-project conventions.
- Every directory name and file placement must have a defensible reason.
- The README must be ready to publish on GitHub without edits.
```

---

### Day 2 — Core Data Schema Design (Logs, Metrics, Traces)

**Intent:** Define the canonical wire-format payloads and storage schemas for all three observability pillars before any ingestion code is written.

```
ROLE: You are a Principal Database and Storage Architect specializing in time-series, 
columnar, and analytical storage engines. You have designed petabyte-scale telemetry 
pipelines at companies processing trillions of data points per month.

PROJECT CONTEXT:
You are defining the foundational data schemas for "OpenTrace", a high-throughput 
observability platform. These schemas are the contract between all producers (SDKs, 
agents) and all consumers (storage engines, query APIs). They must comply with 
OpenTelemetry Semantic Conventions v1.24+.

SCALE REQUIREMENTS:
- Ingest rate: up to 50,000 events/second sustained
- Storage growth: multi-terabyte per day at peak
- Query targets: sub-second aggregations over 30-day windows
- Retention: configurable TTL per pillar (logs: 14 days default, metrics: 90 days, traces: 30 days)

PRIMARY STORAGE: ClickHouse (columnar, MergeTree family)
SECONDARY STORAGE: PostgreSQL 16 with TimescaleDB hypertables

For each of the THREE pillars (Logs, Metrics, Traces), deliver ALL of the following:

PILLAR A — STRUCTURED LOGS

1. JSON Wire Format Schema
   Provide a fully annotated JSON example payload representing a single log event.
   Every field must include: field name, data type, whether required/optional, 
   OTel Semantic Convention reference, and its optimization purpose (e.g., partition key, 
   high-cardinality index target, compression-friendly low-cardinality field).

2. ClickHouse DDL
   - Engine: ReplicatedMergeTree or MergeTree with explicit reasoning
   - PRIMARY KEY and ORDER BY: justify every column included and its sort order
   - PARTITION BY: daily partitions on toYYYYMMDD(timestamp)
   - TTL: automatic row expiry expression
   - CODEC compression hints: LZ4 for bodies, Delta+LZ4 for timestamps, T64 for integers
   - Materialized views or projections for common query patterns (e.g., fast service+level lookup)

3. PostgreSQL + TimescaleDB DDL
   - Hypertable definition with chunk_time_interval
   - JSONB column for flexible attributes
   - GIN index on the JSONB attributes column
   - Composite B-Tree index for (service_name, severity, timestamp DESC)
   - Declarative range partitioning with automated 14-day partition pruning via pg_cron

4. Indexing Trade-off Analysis
   Write a 300-word analysis covering: write amplification cost per added index, 
   how to avoid index bloat on high-cardinality fields like trace_id and user_id, 
   and when BRIN indexes are preferable to B-Tree for time-series append workloads.

PILLAR B — DIMENSIONAL METRICS
[Repeat the same four deliverables as Pillar A, adapted for metrics data: 
 metric_name, metric_type (gauge/counter/histogram), value, labels map, timestamp]

PILLAR C — DISTRIBUTED TRACE SPANS
[Repeat the same four deliverables as Pillar A, adapted for trace spans: 
 trace_id, span_id, parent_span_id, service_name, operation_name, start_time, 
 end_time, status_code, attributes map, events array]

FINAL SECTION — CROSS-PILLAR SCHEMA GOVERNANCE
Describe how all three schemas share a common envelope (resource attributes, 
instrumentation library metadata, schema_url) and how schema versioning is handled 
when field definitions evolve without breaking existing stored data.
```

---

### Day 3 — System Architecture Tradeoffs & Evolution Matrix

**Intent:** Produce a rigorous, interview-grade system design document that exposes all architectural tradeoffs and defines the exact upgrade path from MVP to enterprise scale.

```
ROLE: You are a Staff Distributed Systems Architect presenting a system design defense 
to a senior engineering architecture review board at a company like Stripe or Cloudflare. 
You are required to justify every design decision with concrete failure modes, 
performance data, and operational cost tradeoffs.

PROJECT CONTEXT:
You are architecting "OpenTrace", a high-throughput observability backend targeting 
50,000 events/second at MVP and 500,000+ events/second at enterprise scale. The system 
must achieve high write throughput, sub-100ms query latency for recent data, and 
operator-friendly deployment via Kubernetes.

PRODUCE A COMPLETE SYSTEM DESIGN DOCUMENT with the following sections:

SECTION 1 — COMPONENT BLUEPRINT
For each of the following components, define: operational responsibility boundary, 
stateless or stateful nature, horizontal scalability vector, resource profile 
(CPU-bound / IO-bound / memory-bound), and failure isolation guarantees.
Components: Go SDK, Edge Load Balancer, Collector Service, Redpanda Message Broker, 
Ingestion Worker, ClickHouse Cluster, PostgreSQL, Query API, React UI.

SECTION 2 — ARCHITECTURAL TRADEOFFS (provide brutal, concrete analysis for each)

Tradeoff A — Message Broker vs. Direct Ingestion
Compare: Redpanda/Kafka message queue vs. synchronous HTTP/gRPC direct write to storage.
For each approach, quantify: end-to-end ingest latency (p50/p99), data durability 
guarantees during collector restart, back-pressure propagation behavior, 
operational complexity cost for a 3-engineer team, and the exact request-per-second 
threshold where each approach breaks.
Verdict: State which approach to use for MVP and when to graduate to the other.

Tradeoff B — ClickHouse vs. PostgreSQL for Time-Series Analytics
Compare: ClickHouse MergeTree columnar engine vs. PostgreSQL + TimescaleDB B-Tree engine.
Provide benchmark estimates for: bulk insert throughput (rows/sec), compressed storage 
ratio, GROUP BY aggregation performance on 30-day windows, JOIN complexity for 
cross-pillar correlation queries, and operational maintenance burden (vacuuming, 
compaction, replication setup).
Verdict: Recommend a dual-write strategy or a primary/fallback configuration with 
explicit migration triggers.

Tradeoff C — Pragmatic Monolith vs. Event-Driven Microservices
Compare: a single deployable binary containing collector + ingestion worker + query API 
vs. three independently deployed microservices communicating via Redpanda.
Analyze: deployment complexity, inter-service latency overhead, independent scaling 
capability, blast radius of a single component failure, and debugging complexity.
Verdict: Prescribe the exact team size and traffic threshold that justifies splitting.

SECTION 3 — FAILURE MODES & MITIGATIONS
For each scenario below, describe: the failure trigger, observable symptoms, blast radius, 
and the exact mitigation mechanism to implement.
- Network partition between Collector and Redpanda
- Collector OOM (memory exhaustion from unbounded batch queue)
- Poison pill payload (malformed JSON causing ingestion worker panic)
- ClickHouse backpressure causing ingestion queue saturation
- Hot-spotting on a high-traffic shard key (e.g., single service_name dominates traffic)
- Query API slowdown causing frontend timeouts during heavy analytical queries

SECTION 4 — SCALE EVOLUTION MATRIX
Produce a table with three columns (MVP / Mid-Scale / Enterprise) and the following rows:
Target RPS, Collector replica count, Storage topology, Redpanda partition count, 
Query API replicas, Caching layer, Deployment model, Team size required, 
Monthly infrastructure cost estimate (rough order of magnitude).
Below the table, write a 200-word narrative for each stage describing the specific 
infrastructure changes required to graduate from one stage to the next, including 
the exact metrics thresholds (CPU%, queue lag, p99 latency) that trigger the upgrade.
```

---

### Day 4 — API Contract Design (REST + gRPC)

**Intent:** Define the complete, versioned API contract covering all ingestion and query endpoints before any handler code is written.

```
ROLE: You are a Principal API Architect with experience designing high-throughput 
ingestion APIs and analytical query interfaces at scale (think Datadog Agent API, 
Prometheus remote write, or Honeycomb Events API). You treat the API contract as a 
public commitment to consumers and design for backward compatibility from day one.

PROJECT CONTEXT:
You are designing the complete REST and gRPC API contract for "OpenTrace". The API 
has two distinct planes with different performance characteristics:
- WRITE PLANE: Optimized for throughput, async processing, minimal round-trips
- READ PLANE: Optimized for latency, rich filtering, cursor-based pagination

API VERSION: v1 (design with v2 migration path in mind)
BASE PATH: /api/v1/
AUTHENTICATION: Bearer token via Authorization header (placeholder — auth layer is Phase 3)

DELIVER COMPLETE API SPECIFICATIONS FOR ALL THREE PILLARS:

PILLAR 1 — LOGS API

Endpoint: POST /api/v1/logs
Purpose: Bulk ingestion of structured log events
Request:
- Headers: Content-Type: application/json, Content-Encoding: gzip (optional), 
  X-Service-Token: <token>, X-Request-ID: <uuid>
- Body: JSON array of log event objects (define full schema inline)
- Constraints: Max payload size 5MB (enforced via http.MaxBytesReader), 
  max 1000 events per batch
Response:
- 202 Accepted: async acknowledgment with batch_id and estimated processing time
- 400 Bad Request: RFC 7807 Problem Details for validation failures
- 413 Payload Too Large: with Content-Length limit in response
- 429 Too Many Requests: with X-RateLimit-Limit, X-RateLimit-Remaining, 
  X-RateLimit-Reset, Retry-After headers
- 503 Service Unavailable: when ingestion queue is at capacity

Endpoint: GET /api/v1/logs
Purpose: Filtered retrieval of log events
Query Parameters (define type, validation rules, and default for each):
- service_name (string, exact match)
- level (enum: debug|info|warn|error|fatal)
- start_time (ISO-8601, required)
- end_time (ISO-8601, required, max range 7 days)
- keyword (string, full-text search, max 256 chars)
- cursor (opaque string, keyset pagination token)
- limit (integer, 1-1000, default 100)
Pagination: Cursor/keyset-based using encoded (timestamp, id) tuple — document the 
exact encoding format and how to detect the last page.
Response: 200 OK with data array, next_cursor (null if no more pages), 
total_hint (estimated count, not exact), and query_time_ms.

PILLAR 2 — METRICS API

Endpoint: POST /api/v1/metrics
Purpose: Time-series metric datapoint ingestion
[Define same level of detail as Logs ingestion endpoint]

Endpoint: GET /api/v1/metrics/query
Purpose: Aggregated metric queries with rollup
Query Parameters: metric_name, labels (multi-value), start_time, end_time, 
step (resolution in seconds), aggregation (sum|avg|min|max|p95|p99)
[Define same level of detail as Logs query endpoint]

PILLAR 3 — TRACES API

Endpoint: POST /api/v1/traces
Purpose: Distributed trace span ingestion (streaming batches)
[Define same level of detail as Logs ingestion endpoint]

Endpoint: GET /api/v1/traces/{trace_id}
Purpose: Full trace reconstruction with dependency graph
Response: Root span, all child spans in parent-child tree structure, 
critical path highlighting, total trace duration, per-service timing breakdown.

SECTION: VERSIONING & BACKWARD COMPATIBILITY STRATEGY
Define how the API handles evolution from v1 to v2:
- URL versioning strategy (/api/v1/ vs /api/v2/)
- Field addition (always backward compatible) vs. field removal (requires deprecation cycle)
- Sunset header usage (Sunset: <RFC-date>) for deprecated endpoints
- Client SDK versioning alignment with API version changes
- How to run v1 and v2 simultaneously during migration windows

SECTION: gRPC SERVICE DEFINITION
Provide a complete .proto file (proto3) for the ingestion service covering all three 
pillars with streaming RPC for high-throughput write paths and unary RPC for queries. 
Include service metadata comments and field number reservation strategy for future evolution.
```

---

### Day 5 — Docker + Local Development Environment

**Intent:** Create a fully deterministic, production-mirroring local development stack that any engineer can spin up in under five minutes.

```
ROLE: You are a Lead DevOps and Platform Engineer with expertise in containerized 
development environments. You design local stacks that mirror production topology 
closely enough that "works on my machine" failures are eliminated before code reaches CI.

PROJECT CONTEXT:
You are building the complete local development environment for "OpenTrace". The stack 
must faithfully simulate production networking, resource constraints, service dependencies, 
and startup ordering. All engineers — regardless of OS — must get an identical, 
reproducible environment from a single command: docker compose up.

TARGET SERVICES AND THEIR ROLES:
- collector-service (Go): Receives telemetry from SDKs, publishes to Redpanda
- query-api (Go): Reads from ClickHouse/PostgreSQL, serves the React frontend
- opentrace-ui (React/Nginx): Single-page app served via Nginx
- postgres (PostgreSQL 16): Relational store for logs and metadata
- clickhouse (ClickHouse 24.x): Columnar analytics store for metrics and traces
- redpanda (Redpanda latest): Kafka-compatible message broker for ingestion queue
- redpanda-console (optional): Web UI for inspecting Redpanda topics

DELIVERABLE 1 — docker-compose.yml
Write a complete, production-quality docker-compose.yml (Compose Specification format, 
not legacy v2/v3 syntax). Requirements:
- All service images pinned to specific digest or exact version tags (no :latest)
- Custom named bridge networks: backend-net (DB + brokers + APIs, not exposed externally) 
  and frontend-net (UI + query-api only, expose ports 80/443)
- Explicit depends_on with condition: service_healthy for all dependency ordering
- Deep health check commands for each service:
  * postgres: pg_isready -U ${POSTGRES_USER} -d ${POSTGRES_DB}
  * clickhouse: clickhouse-client --query "SELECT 1"
  * redpanda: rpk cluster health
  * collector-service: curl -f http://localhost:8080/healthz
  * query-api: curl -f http://localhost:8081/healthz
- Resource limits: deploy.resources.limits (memory + cpus) for each container
- Named volumes for all persistent data (postgres_data, clickhouse_data, redpanda_data)
- All configuration via environment variables referencing .env file (no hardcoded values)

DELIVERABLE 2 — Dockerfiles

Dockerfile for Go services (collector-service and query-api):
- Multi-stage build: stage 1 (golang:1.22-alpine) for compilation, 
  stage 2 (gcr.io/distroless/static or scratch) for final image
- Explicit GOOS=linux GOARCH=amd64 build flags
- Non-root USER (numeric UID, e.g., 65532)
- --mount=type=cache for Go module and build caches to speed up CI rebuilds
- Binary compiled with -trimpath -ldflags="-s -w" for minimal image size
- Final image must be under 20MB

Dockerfile for React UI (opentrace-ui):
- Multi-stage build: stage 1 (node:20-alpine) for npm build, 
  stage 2 (nginx:1.26-alpine) for serving
- Nginx config for SPA routing (try_files $uri $uri/ /index.html)
- Non-root USER for Nginx
- Cache-busting headers for static assets (Cache-Control: max-age=31536000, immutable)
- Health check endpoint: /health returning 200 OK

DELIVERABLE 3 — .env.example
Provide a complete, commented .env.example file covering all required variables:
- Database credentials (POSTGRES_USER, POSTGRES_PASSWORD, POSTGRES_DB, POSTGRES_HOST)
- ClickHouse connection (CLICKHOUSE_HOST, CLICKHOUSE_PORT, CLICKHOUSE_DB)
- Redpanda broker addresses (REDPANDA_BROKERS)
- Service ports (COLLECTOR_PORT, QUERY_API_PORT, UI_PORT)
- Application config (LOG_LEVEL, MAX_BATCH_SIZE, COLLECTOR_ENDPOINT)
- Environment tag (ENVIRONMENT=development)
Group variables by service with section comments.

DELIVERABLE 4 — Makefile targets
Provide a Makefile with targets: up, down, logs, reset (destroys volumes), 
build (forces image rebuild), test-health (curls all health endpoints and reports status).
```

---

### Day 6 — Go Microservice Scaffolding (Collector + Query API)

**Intent:** Produce compile-ready, idiomatic Go skeletons for both backend services that enforce production patterns from the first commit.

```
ROLE: You are a Staff Go Systems Engineer with experience building high-throughput 
network services at scale. You write Go code that is idiomatic, observable, 
and safe by default — not just "functional". You treat every service skeleton 
as the architectural template junior engineers will follow for all future code.

PROJECT CONTEXT:
Scaffold two decoupled Go 1.22+ microservices for the "OpenTrace" platform:

SERVICE 1: collector-service
- Purpose: Receive telemetry payloads from SDKs via HTTP, validate, and enqueue to Redpanda
- Performance profile: High throughput (50k+ requests/sec target), 
  non-blocking, minimal per-request allocations
- Port: 8080

SERVICE 2: query-api
- Purpose: Serve filtered log/metric/trace queries to the React frontend
- Performance profile: Low latency (p99 < 50ms for indexed queries), 
  connection-pooled reads from ClickHouse and PostgreSQL
- Port: 8081

FOR EACH SERVICE, produce the following files — provide complete, compilable code 
(no pseudo-code, no TODO stubs except where explicitly noted):

FILE STRUCTURE REQUIRED:
  cmd/<service-name>/main.go          — binary entry point, signal handling, server lifecycle
  internal/server/server.go           — HTTP server construction, route registration, middleware chain
  internal/handler/health.go          — /healthz endpoint implementation
  internal/handler/ingest.go          — primary business logic handler (collector) or query.go (query-api)
  internal/middleware/logging.go      — structured request logging middleware
  internal/middleware/recovery.go     — panic recovery middleware
  internal/middleware/requestid.go    — request ID injection middleware
  internal/config/config.go           — environment-based configuration loading

IMPLEMENTATION REQUIREMENTS:

main.go:
- Use chi v5 router (or net/http ServeMux with Go 1.22 method+path patterns)
- HTTP server with explicit timeouts: ReadTimeout 5s, WriteTimeout 10s, 
  IdleTimeout 120s, ReadHeaderTimeout 2s
- Graceful shutdown: catch SIGINT/SIGTERM, call server.Shutdown(ctx) with 30s timeout
- Structured startup logging via log/slog (JSON format in production, text in development)
- Load config from environment at startup, fail fast on missing required variables

config.go:
- Use os.LookupEnv for all config values with explicit defaults and required field validation
- Define a Config struct with all fields typed (no raw string maps)
- Provide a Load() function that returns (*Config, error) — never panics
- Log all loaded config values at startup (redacting sensitive fields like passwords)

Middleware chain (apply in this order):
1. requestid: generate UUID v4 request ID, set in context and X-Request-ID response header
2. logging: structured slog entry per request with: method, path, status, duration_ms, 
   request_id, remote_addr — use slog.With to attach request_id to all log lines in scope
3. recovery: catch panics, log stack trace via slog.Error with request_id, 
   return RFC 7807 JSON problem response with status 500

/healthz endpoint:
- Returns JSON: {"status": "ok", "version": "<build_version>", "uptime_seconds": <n>, 
  "checks": {"database": "ok|degraded|down", "broker": "ok|degraded|down"}}
- Return 200 if all checks pass, 503 if any check is "down"
- Perform actual dependency probes (pg ping, clickhouse ping) — stub with TODO for now
- Include build version injected at compile time via -ldflags -X flag

CODING STANDARDS:
- All errors wrapped with fmt.Errorf("context: %w", err) — never silently dropped
- No global variables except for build-time injected version strings
- All HTTP handlers receive a *http.Request with context — pass context to all downstream calls
- Use httptest.NewRecorder in table-driven unit tests for all handlers (provide one example test per handler)
```

---

### Day 7 — Pre-Implementation Architecture Review

**Intent:** Force a rigorous critique of all Phase 0 decisions before writing production code, while corrections are still cheap.

```
ROLE: You are a Principal Infrastructure Engineer and Technical Fellow at a company 
operating observability infrastructure at the scale of Grafana Labs or Elastic. 
You are conducting a pre-production architecture review with the authority to block 
the implementation phase until critical issues are resolved.

REVIEW SUBJECT:
I am presenting my Phase 0 architectural decisions for "OpenTrace" for your critique.

[PASTE YOUR ACCUMULATED DESIGN ARTIFACTS HERE: folder structure, schema DDLs, 
 API contract, docker-compose, and service skeleton designs from Days 1-6]

YOUR MANDATE:
Provide a comprehensive, unsparing architectural critique. Do not soften findings. 
Every gap you miss will become a production incident.

REVIEW CATEGORIES — analyze each independently:

CATEGORY 1 — OVER-ENGINEERING
Identify components, abstractions, or design patterns that introduce operational 
complexity disproportionate to current scale requirements (0–5k events/sec MVP).
For each finding: name the component, explain why it is premature, quantify the 
operational overhead it adds, and prescribe the simpler alternative.

CATEGORY 2 — UNDER-DESIGNED SUBSYSTEMS
Identify missing reliability mechanisms that will cause data loss or system failure 
in production. Check specifically for:
- Missing circuit breakers between the collector and Redpanda
- Missing rate limiting on ingest endpoints
- Missing dead-letter queue strategy for unprocessable messages
- Missing connection pool limits that could exhaust database connections
- Missing request body size limits enabling OOM via oversized payloads
- Single points of failure with no redundancy path
For each finding: describe the exact failure scenario, its blast radius, and the fix.

CATEGORY 3 — SCALE TRIGGER POINTS
Identify the exact bottleneck that will cause each component to fail as load increases. 
For each, specify:
- The metric that will hit its limit first (e.g., PostgreSQL WAL write throughput)
- The approximate load level (events/sec or GB/day) where the limit is reached
- Whether the fix requires a config change, horizontal scaling, or architectural redesign

CATEGORY 4 — REMEDIATION BLUEPRINT
Produce a prioritized remediation matrix with three severity tiers:
- CRITICAL (fix before writing any Phase 1 code): issues that guarantee production failure
- WARNING (fix within two weeks of MVP launch): issues that degrade reliability under load
- OPTIMIZATION (fix before reaching 10k events/sec): performance improvements that matter at scale
Format as a table: | Severity | Component | Issue | Recommended Fix | Effort (S/M/L) |

End with a Go/No-Go recommendation for proceeding to Phase 1, with any blocking conditions stated explicitly.
```

---

## Phase 1 — Log Ingestion MVP

---

### Day 8 — Log Ingestion HTTP Endpoint

**Intent:** Implement the production-hardened `POST /v1/logs` handler with complete input validation, DoS protection, and structured error responses.

```
ROLE: You are a Senior Go Engineer specializing in high-throughput HTTP service 
implementation. You write handlers that are safe by default — protecting against 
malformed input, oversized payloads, and malicious clients without degrading 
performance for legitimate traffic.

PROJECT CONTEXT:
Implement the POST /api/v1/logs endpoint for the OpenTrace collector-service. 
This endpoint is the primary ingest path for all structured log events from SDK clients. 
It must handle up to 1,000 log events per batch, validate all required fields, 
and return RFC 7807-compliant error responses for any schema violation.

FILE: internal/handler/logs_ingest.go

STEP 1 — DATA STRUCTURES
Define Go structs for the complete log ingestion payload:

type IngestLogsRequest struct { ... }  // top-level wrapper with metadata
type LogEvent struct { ... }           // individual log event
type ResourceAttributes struct { ... } // OTel resource attributes (service.name, host.name, etc.)
type LogAttributes struct { ... }      // arbitrary key-value event attributes

Requirements:
- Use json struct tags with omitempty where fields are optional
- Timestamp field: use string type for parsing, validate ISO-8601 format in validation layer
- LogLevel: define as a custom string type with a Validate() method checking allowed values
- Provide a custom UnmarshalJSON on LogEvent if any field requires transformation on parse
- Use pointer types (*string) for optional fields to distinguish "not provided" from empty string
- Define a const block for allowed log level values: DEBUG, INFO, WARN, ERROR, FATAL

STEP 2 — HANDLER IMPLEMENTATION
func HandleIngestLogs(w http.ResponseWriter, r *http.Request) — requirements:
- Wrap request body with http.MaxBytesReader(w, r.Body, 5*1024*1024) before any read
- Decode JSON using json.NewDecoder(body).Decode(&req) — do NOT use json.Unmarshal(io.ReadAll(...))
- Propagate request context (r.Context()) to all downstream calls
- After successful validation, call a LogRepository interface method (stub the interface)
- Log a structured slog entry on successful ingest: batch_size, request_id, duration_ms
- On error, call a writeError helper that always produces RFC 7807 Problem Details JSON

STEP 3 — VALIDATION ENGINE
Write a validate(req *IngestLogsRequest) []ValidationError function:
- Check: len(req.Events) > 0 and <= 1000
- Check: each LogEvent.Timestamp parses as RFC3339 (time.Parse(time.RFC3339, ...))
- Check: each LogEvent.Level is one of the allowed enum values
- Check: LogEvent.Message is non-empty and <= 32,768 characters
- Check: LogEvent.ResourceAttributes.ServiceName is non-empty and <= 255 characters
- Check: total attribute key count per event <= 100
- Collect ALL validation errors before returning (not fail-fast) to give clients complete feedback
- Return a slice of ValidationError{Field: "events[2].level", Message: "invalid value: VERBOSE"}

STEP 4 — ERROR RESPONSE HELPER
func writeError(w http.ResponseWriter, status int, title string, detail string, 
                violations []ValidationError):
- Set Content-Type: application/problem+json
- Write RFC 7807 body: {"type": "...", "title": "...", "status": 400, 
  "detail": "...", "violations": [...]}
- For 400 errors caused by MaxBytesReader, emit a distinct error type URI and 413 status

STEP 5 — UNIT TESTS (file: internal/handler/logs_ingest_test.go)
Write table-driven tests using httptest.NewRecorder covering:
- Valid batch of 3 log events → 202 Accepted
- Empty events array → 400 with violations
- Invalid log level → 400 with field path in violations
- Payload exceeding 5MB → 413
- Non-JSON body → 400
- Missing required service_name → 400 with specific field identified
```

---

### Day 9 — PostgreSQL Schema for Logs

**Intent:** Design a hardened, partitioned PostgreSQL schema engineered specifically for high-velocity log append workloads with sub-second query performance.

```
ROLE: You are a Principal Database Engineer who has designed PostgreSQL schemas for 
logging systems processing billions of rows per day. You treat schema design as a 
performance contract — every column type, index, and partition decision has a 
measurable impact on write throughput and query latency.

PROJECT CONTEXT:
Design the PostgreSQL 16 schema for OpenTrace's structured log storage layer. 
This schema must support:
- Write throughput: 50,000+ log events per second via bulk insert
- Query patterns: filter by service_name, level, time range, and keyword in message
- Retention: automated 14-day rolling window via partition pruning
- Scale target: 50+ million rows per partition before degradation

DELIVERABLE 1 — CORE SCHEMA (complete SQL script)

Create the following objects in order:

a) Enum type for log severity:
   CREATE TYPE log_severity AS ENUM ('debug', 'info', 'warn', 'error', 'fatal');

b) Main logs table with declarative range partitioning by day:
   Columns required (with exact PostgreSQL types and reasoning):
   - id: BIGINT GENERATED ALWAYS AS IDENTITY (not UUID — avoid random index fragmentation)
   - trace_id: UUID nullable (foreign key correlation to traces pillar)
   - span_id: UUID nullable
   - timestamp: TIMESTAMPTZ NOT NULL (partition key and primary sort dimension)
   - received_at: TIMESTAMPTZ NOT NULL DEFAULT now() (ingest lag measurement)
   - service_name: TEXT NOT NULL (low-cardinality, frequent filter)
   - severity: log_severity NOT NULL
   - severity_text: TEXT (original string from SDK before enum mapping)
   - body: TEXT NOT NULL (the log message)
   - resource_attributes: JSONB (OTel resource attributes: host.name, k8s.pod.name, etc.)
   - log_attributes: JSONB (event-level custom attributes)
   - schema_url: TEXT

   Partition by RANGE (timestamp), daily intervals.

c) Automated partition creation procedure:
   Write a PL/pgSQL function create_log_partition(date) that creates the partition 
   for a given day if it does not already exist. Show how to call this from pg_cron 
   to pre-create tomorrow's partition at 23:50 each night.

d) Automated partition pruning procedure:
   Write a PL/pgSQL function drop_old_log_partitions(retention_days INT) that 
   finds and drops partitions older than retention_days using pg_inherits and 
   partition boundary metadata. Show the pg_cron schedule.

DELIVERABLE 2 — INDEXING STRATEGY

For each index, provide: CREATE INDEX statement, the query pattern it satisfies, 
and an explanation of why this specific index type was chosen.

Required indexes:
1. Composite B-Tree on (service_name, severity, timestamp DESC) — primary query filter
2. Composite B-Tree on (timestamp DESC, id DESC) — keyset pagination anchor
3. GIN on log_attributes — arbitrary JSONB key-value search
4. GIN on to_tsvector('english', body) — full-text keyword search on message body
5. BRIN on received_at — low-cost sequential scan optimization for recent data queries

All indexes must be created with CREATE INDEX CONCURRENTLY to allow creation without 
table locking. Show the correct approach for applying these to a partitioned table 
(indexes on parent table cascade to partitions in PostgreSQL 16).

DELIVERABLE 3 — WRITE AMPLIFICATION ANALYSIS
Write a 400-word technical brief covering:
- How many index updates are triggered per single log INSERT
- Estimated throughput degradation per additional index added (reference PG benchmark data)
- Autovacuum configuration tuning for append-heavy tables 
  (autovacuum_vacuum_scale_factor, autovacuum_vacuum_cost_delay)
- Why fillfactor=90 on the main table improves HOT update efficiency for JSONB columns
- The exact point (rows per day, index size GB) where BRIN becomes more efficient than B-Tree 
  for the received_at column

DELIVERABLE 4 — MIGRATION SCRIPT
Write a complete, idempotent migration SQL file (001_create_logs_schema.sql) that:
- Uses CREATE TABLE IF NOT EXISTS and CREATE INDEX IF NOT EXISTS patterns
- Creates the first 30 daily partitions covering today through today+29
- Includes COMMENT ON TABLE and COMMENT ON COLUMN for all columns
- Can be run repeatedly without error (idempotent)
```

---

### Day 10 — PostgreSQL Persistence Layer in Go

**Intent:** Implement a production-grade, injection-safe database layer that connects the ingest handler to PostgreSQL using modern pgx patterns.

```
ROLE: You are a Senior Go Backend Engineer with deep expertise in PostgreSQL driver 
internals, connection pool tuning, and bulk write optimization. You treat the database 
layer as a performance-critical infrastructure component, not application glue code.

PROJECT CONTEXT:
Implement the PostgreSQL persistence layer for the OpenTrace collector-service 
log ingestion pipeline. This layer receives validated log event batches from the 
HTTP handler and writes them durably to PostgreSQL using the jackc/pgx/v5 driver.

Target write throughput: 50,000 events/second via connection pool with bulk insert.
Zero tolerance for SQL injection vulnerabilities.
All database errors must be classified and wrapped with context before propagation.

FILE: internal/repository/logs_postgres.go

STEP 1 — CONNECTION POOL MANAGER (internal/database/pool.go)

Implement func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error):
- Use pgxpool.ParseConfig to parse DSN from config
- Set pool parameters explicitly (do not rely on defaults):
  * MaxConns: 20 (calculate based on PostgreSQL max_connections / number of service replicas)
  * MinConns: 2 (keep warm connections ready)
  * MaxConnIdleTime: 5 * time.Minute
  * MaxConnLifetime: 30 * time.Minute
  * MaxConnLifetimeJitter: 1 * time.Minute (prevent thundering herd on simultaneous expiry)
  * HealthCheckPeriod: 1 * time.Minute
- Set BeforeAcquire hook to verify connection health
- Verify pool on startup with pool.Ping(ctx) — return error if database unreachable
- Log pool configuration at INFO level on successful initialization
- Return the pool — the caller owns its lifecycle and calls pool.Close() on shutdown

STEP 2 — REPOSITORY INTERFACE

Define the interface in internal/repository/logs.go:
type LogRepository interface {
    BulkInsert(ctx context.Context, events []model.LogEvent) error
    Close()
}
This enables mock injection in unit tests.

STEP 3 — BULK INSERT IMPLEMENTATION

Implement PostgresLogRepository.BulkInsert using pgx CopyFrom:
- Use pgx.CopyFromRows with a custom pgx.CopyFromSource implementation that iterates 
  over []model.LogEvent without allocating an intermediate [][]interface{} slice
- Column list must exactly match the table DDL from Day 9
- Map LogEvent.Timestamp string to time.Time before insert using time.Parse(time.RFC3339, ...)
- Marshal log_attributes map to JSONB using pgtype.JSONB or json.Marshal
- Wrap in a context-aware transaction: begin → copy → commit, with rollback on error
- Measure and log insert duration and row count at DEBUG level

STEP 4 — ERROR CLASSIFICATION

Implement func classifyPgError(err error) error:
- Use errors.As(err, &pgErr) to extract *pgconn.PgError
- Classify by SQLState code:
  * 23xxx (integrity constraint) → ErrConstraintViolation
  * 40xxx (serialization/deadlock) → ErrRetryable  
  * 53xxx (insufficient resources) → ErrResourceExhausted
  * 57xxx (operator intervention/cancel) → ErrCancelled
  * All others → ErrInternal
- Wrap with original error for full stack: fmt.Errorf("postgres: %w: %w", classified, pgErr)
- Log pgerr.Code, pgerr.Message, pgerr.Detail at ERROR level with request context

STEP 5 — GRACEFUL SHUTDOWN
The repository must implement io.Closer. Close() method:
- Waits for any in-flight BulkInsert calls to complete (use sync.WaitGroup)
- Calls pool.Close() after all operations finish
- Logs "repository closed cleanly" or "repository closed with N in-flight operations abandoned"

STEP 6 — UNIT TESTS (internal/repository/logs_postgres_test.go)
Use pgxmock v2 to mock the pgx connection. Write tests for:
- Successful bulk insert of 100 events → verify correct column count and row count
- CopyFrom returning a constraint error → verify ErrConstraintViolation is returned
- Context cancellation mid-insert → verify rollback is called and context.Canceled returned
- Zero-length events slice → verify early return without database call
```

---

### Day 11 — Log Query API Endpoint

**Intent:** Implement a production-hardened `GET /v1/logs` handler with dynamic filtering, injection-safe query building, and O(1) keyset pagination.

```
ROLE: You are an API Performance Engineer specializing in building analytical query 
interfaces over large relational datasets. You treat every query endpoint as a 
potential full-table-scan waiting to happen, and you design against it from the start.

PROJECT CONTEXT:
Implement the GET /api/v1/logs endpoint for the OpenTrace query-api service. 
This endpoint must support multidimensional filtering over tens of millions of log rows 
with consistent sub-100ms response times regardless of page depth.

FILE: internal/handler/logs_query.go

STEP 1 — QUERY PARAMETER PARSER

Implement func parseLogsQueryParams(r *http.Request) (*LogsQueryParams, error):
struct LogsQueryParams {
    ServiceName  *string    // optional exact match
    Level        *string    // optional enum match
    StartTime    time.Time  // required, parse RFC3339
    EndTime      time.Time  // required, parse RFC3339, must be after StartTime
    Keyword      *string    // optional full-text search, max 256 chars
    Cursor       *LogsCursor // decoded from opaque cursor string
    Limit        int        // 1-1000, default 100
}

Validation rules:
- StartTime and EndTime are required — return 400 if missing
- EndTime - StartTime must not exceed 7 days — return 400 with clear message
- Level must be one of the enum values or absent — return 400 for invalid values
- Limit must be 1-1000 — clamp silently or return 400 (choose one, document the choice)
- Cursor string must base64-decode to a valid (timestamp, id) pair — return 400 if malformed

STEP 2 — KEYSET CURSOR ENCODING

Implement the cursor encode/decode pair:
type LogsCursor struct {
    Timestamp time.Time
    ID        int64
}
func EncodeCursor(c LogsCursor) string  — base64url encode JSON of {ts, id}
func DecodeCursor(s string) (*LogsCursor, error) — inverse, return error on invalid input

The cursor represents the last seen (timestamp, id) from the previous page. The next 
page query uses: WHERE (timestamp, id) > (cursor.Timestamp, cursor.ID)

STEP 3 — DYNAMIC QUERY BUILDER

Implement func buildLogsQuery(p *LogsQueryParams) (string, []interface{}, error):
- Use a string builder to construct the WHERE clause with numbered placeholders ($1, $2, ...)
- NEVER concatenate user-supplied strings directly into the query
- Always include: timestamp >= $n AND timestamp <= $n as the base time range filter
- Conditionally append: AND service_name = $n, AND severity = $n::log_severity, 
  AND body_tsv @@ plainto_tsquery('english', $n), AND (timestamp, id) > ($n, $n) for cursor
- Always append: ORDER BY timestamp ASC, id ASC LIMIT $n
- Return the query string and the args slice with correct positional alignment
- Write a unit test that verifies query building for every parameter combination 
  without a real database (test the SQL string and args, not execution)

STEP 4 — HANDLER IMPLEMENTATION

func HandleQueryLogs(repo LogReadRepository) http.HandlerFunc:
- Parse and validate query params (return 400 on error with RFC 7807 body)
- Call repo.QueryLogs(ctx, params) to fetch results
- If results length == limit, encode the last row as next_cursor; else next_cursor = null
- Return JSON: {"data": [...], "next_cursor": "...|null", "query_time_ms": 42}
- Set Cache-Control: no-store (observability data must be fresh)
- Add X-Query-Time-Ms response header for client-side monitoring

STEP 5 — REPOSITORY READ INTERFACE

Define in internal/repository/logs.go (extend the interface from Day 10):
QueryLogs(ctx context.Context, params *LogsQueryParams) ([]model.LogEvent, error)

Implement using pgxpool.Pool.Query with rows.Scan into model.LogEvent structs.
Scan all columns explicitly — do not use SELECT * — list every column by name.
```

---

### Day 12 — Query Performance Optimization & Index Tuning

**Intent:** Produce a definitive indexing and query tuning guide that eliminates sequential scans and ensures predictable performance under production load.

```
ROLE: You are an Elite PostgreSQL Performance Engineer and Query Optimizer. 
You have tuned PostgreSQL installations handling billions of rows and you treat 
EXPLAIN ANALYZE output as the ground truth for all performance decisions.

PROJECT CONTEXT:
The OpenTrace logs query endpoint (GET /api/v1/logs) executes dynamic multi-filter 
queries against a PostgreSQL table that will grow to 500M+ rows across partitions. 
We need to verify our index strategy eliminates sequential scans and provides 
predictable sub-50ms query performance under concurrent load.

DELIVERABLE 1 — COMPOSITE INDEX DEFINITIONS

For each of the following query patterns, provide:
a) The exact CREATE INDEX statement optimized for that pattern
b) Why the column order within the composite index matters for this specific pattern
c) Whether the query can use an Index Only Scan (explain why/why not)

Query patterns to cover:
1. Filter by service_name + severity + time range (the most common dashboard query)
2. Filter by time range only (full-service dashboard view)
3. Filter by service_name + time range + keyword in body (full-text search path)
4. Keyset pagination: WHERE (timestamp, id) > (cursor_ts, cursor_id)
5. Aggregate query: COUNT(*) GROUP BY severity for a single service over 1 hour

DELIVERABLE 2 — EXPLAIN ANALYZE INTERPRETATION GUIDE

Provide annotated EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT) output for the primary 
multi-filter query:
  SELECT id, timestamp, service_name, severity, body, log_attributes
  FROM logs
  WHERE service_name = 'payment-service'
    AND severity = 'error'
    AND timestamp BETWEEN '2024-01-01T00:00:00Z' AND '2024-01-01T01:00:00Z'
  ORDER BY timestamp ASC, id ASC
  LIMIT 100;

Show two versions:
a) BEFORE optimal indexing: annotate the Seq Scan nodes and quantify the cost
b) AFTER optimal indexing: annotate the Index Scan or Index Only Scan nodes

For each plan node, explain: what the cost numbers mean, how to interpret Buffers 
(shared hit vs read), and what "Rows Removed by Filter" indicates about index selectivity.

DELIVERABLE 3 — COMMON PERFORMANCE PITFALLS RUN-BOOK

For each pitfall, provide: symptom (what you see in EXPLAIN), root cause, and the fix.

1. TIMEZONE CONVERSION INDEX INVALIDATION
   Symptom: Query uses AT TIME ZONE conversion in WHERE clause, causing index miss.
   Root cause and fix: store all timestamps as TIMESTAMPTZ and always query in UTC.

2. NULL HANDLING IN COMPOSITE INDEXES
   Symptom: Queries filtering on nullable columns miss the index.
   Root cause: NULLs sort differently, causing index mismatch.
   Fix: Use IS NOT DISTINCT FROM or create partial indexes on non-null subsets.

3. COLUMN ORDER TRAP IN COMPOSITE B-TREE
   Symptom: Index exists but query uses Bitmap Heap Scan instead of Index Scan.
   Root cause: Query filters on the second column without filtering on the first.
   Fix: Reorder composite index to match the most selective filter first.

4. JSONB ATTRIBUTE QUERY WITHOUT GIN INDEX
   Symptom: Full table scan when filtering on log_attributes->>'user_id'.
   Fix: GIN index with jsonb_path_ops operator class, and query rewrite to use @> operator.

5. STALE STATISTICS CAUSING BAD PLAN CHOICES
   Symptom: Planner estimates 1 row but actual rows = 500,000.
   Fix: ANALYZE the table after large bulk loads, tune statistics target for high-cardinality columns.

DELIVERABLE 4 — AUTOMATED PERFORMANCE MONITORING QUERIES
Write 5 SQL queries that can be run as a daily health check:
1. Find all sequential scans on the logs table over the past 24 hours
2. Find indexes with zero scans in the past 7 days (index bloat candidates)
3. Find queries with the highest total execution time (pg_stat_statements)
4. Find tables with high autovacuum lag (dead tuple accumulation)
5. Find partition sizes and row counts for the past 14 days
```

---

### Day 13 — Definitive Indexing Strategy (Write vs. Read Balance)

**Intent:** Produce an analytical framework that quantifies the exact performance cost of each index choice and prescribes an optimal configuration for mixed read/write workloads.

```
ROLE: You are a Principal Infrastructure Architect who has managed PostgreSQL 
deployments at petabyte scale. You understand that every index is a write tax, 
and you design indexing strategies as deliberate performance contracts between 
ingestion throughput and query latency.

PROJECT CONTEXT:
OpenTrace's PostgreSQL log storage must simultaneously support:
- WRITE PATH: Bulk inserts of 50,000 events/second via pgx CopyFrom
- READ PATH: Sub-50ms filter queries with cursor pagination from the React dashboard

We need a definitive, mathematically grounded indexing strategy that maximizes write 
throughput while maintaining acceptable read performance, with explicit escalation 
triggers as data volume grows.

DELIVERABLE 1 — INDEX TYPE SELECTION FRAMEWORK

For each index type below, provide a decision matrix answering: 
"When should I use this for OpenTrace telemetry data?"

B-TREE INDEX:
- Optimal cardinality range: when to use vs. avoid
- Write cost: approximate WAL amplification factor per insert (e.g., 1.3x–2x)
- Best columns in our schema: list specific column names and why
- When to use partial B-Tree (e.g., CREATE INDEX ... WHERE severity = 'error')

BRIN INDEX (Block Range INdex):
- Why BRIN is highly effective for the timestamp column specifically
- Size comparison: BRIN vs B-Tree for 100M timestamp values (approximate bytes)
- Write cost: why BRIN adds nearly zero write overhead
- Limitation: why BRIN cannot replace B-Tree for service_name or severity queries

GIN INDEX:
- Why GIN is required for JSONB attributes and full-text search
- Write cost: GIN is significantly more expensive than B-Tree — quantify the overhead
- gin_pending_list_limit tuning: how to configure pending list size to reduce write stalls
- When to use jsonb_ops vs jsonb_path_ops operator class

DELIVERABLE 2 — WRITE THROUGHPUT IMPACT QUANTIFICATION

Produce a table showing the theoretical impact of each index configuration on bulk insert throughput:

| Configuration | Estimated Max Insert Throughput | WAL Size per 1M Rows | Notes |
|---|---|---|---|
| No indexes | ~500K rows/sec | baseline | |
| + B-Tree on (timestamp) | ... | ... | ... |
| + B-Tree on (service_name, severity, timestamp) | ... | ... | ... |
| + GIN on log_attributes | ... | ... | ... |
| + GIN on body full-text | ... | ... | ... |
| Full recommended config | ... | ... | ... |

Reference PostgreSQL internals: every B-Tree insert requires a random IO to the index 
leaf page; GIN inserts to a pending list (sequential) but triggers merge passes. 
Quantify these with approximate IOPS numbers.

DELIVERABLE 3 — MAINTENANCE AND PRUNING RUN-BOOK

Provide complete SQL scripts for the following maintenance operations:

a) REINDEX WITHOUT DOWNTIME
   Script using REINDEX INDEX CONCURRENTLY — explain why this is required for 
   index bloat recovery on heavily-written tables, and the appropriate pg_cron schedule.

b) INDEX FRAGMENTATION ASSESSMENT
   SQL query using pg_stat_user_indexes and pgstattuple extension to measure 
   index bloat percentage. Define the threshold (e.g., >30% bloat) that triggers reindex.

c) UNUSED INDEX DETECTION AND REMOVAL
   SQL query joining pg_stat_user_indexes with pg_indexes to find indexes with 
   zero idx_scan count. Provide the safe process: document → schedule removal window → 
   DROP INDEX CONCURRENTLY → verify query plans unchanged.

d) STATISTICS MAINTENANCE
   ALTER TABLE logs ALTER COLUMN service_name SET STATISTICS 500; — explain why 
   increasing statistics targets improves planner accuracy for high-cardinality columns, 
   and which columns in our schema warrant higher statistics targets.

DELIVERABLE 4 — RECOMMENDED FINAL INDEX CONFIGURATION
Output the complete, production-recommended set of CREATE INDEX CONCURRENTLY statements 
in execution order, with a one-line comment on each explaining its purpose and the 
query pattern it primarily serves.
```

---

### Day 14 — End-to-End Automated Testing Suite

**Intent:** Design and implement a complete testing matrix covering correctness, failure resilience, and throughput validation for the entire log ingestion stack.

```
ROLE: You are a Principal QA and Chaos Engineering specialist with experience building 
test harnesses for distributed observability pipelines. You treat tests as executable 
specifications — they must catch regressions, simulate real failure modes, and validate 
performance SLOs, not just confirm happy paths.

PROJECT CONTEXT:
Design and implement the complete automated testing suite for the OpenTrace log ingestion 
MVP covering: Go collector-service, PostgreSQL persistence layer, and query-api endpoint.
The test suite must be runnable in CI with docker compose and must validate both 
correctness and performance characteristics.

DELIVERABLE 1 — UNIT TEST SUITE (Go testing package)

Write table-driven unit tests for the following components:

a) Log event validation (internal/handler/logs_ingest_test.go):
   Test matrix covering every validation rule from Day 8:
   - Valid event: 202 expected
   - Empty events array: 400 with specific violation message
   - Events array length > 1000: 400 with specific violation
   - Invalid timestamp format: 400 with field path "events[0].timestamp"
   - Invalid log level "VERBOSE": 400 with field path "events[0].level"
   - Message length > 32768 chars: 400 with field path "events[0].message"
   - Payload > 5MB: 413
   - Valid events with all optional fields absent: 202 (test omitempty behavior)

b) Query parameter parsing (internal/handler/logs_query_test.go):
   - Valid params with all filters: verify correct struct values
   - Missing start_time: 400
   - Time range > 7 days: 400 with clear message
   - Invalid cursor string (not valid base64): 400
   - Limit = 0: verify clamping or 400 behavior
   - Limit = 1001: verify clamping or 400 behavior

c) Cursor encode/decode round-trip:
   - EncodeCursor(DecodeCursor(EncodeCursor(cursor))) == original cursor
   - Tampered cursor string returns decode error

DELIVERABLE 2 — INTEGRATION TEST SUITE (testcontainers-go)

Write integration tests that spin up a real PostgreSQL container:

a) BulkInsert integration test:
   - Insert 10,000 log events in a single CopyFrom call
   - Verify row count via SELECT COUNT(*)
   - Verify a sample row's field values match the input exactly
   - Verify timestamp is stored correctly (UTC, no tz drift)
   - Verify log_attributes JSONB round-trips without data loss

b) Query integration test:
   - Seed 1,000 events across 3 services, 5 severity levels, over 2 hours
   - Execute 10 different filter combinations and verify result counts match expected
   - Verify keyset pagination: page through all 1,000 results 100 at a time, 
     confirm no duplicates and no gaps (collect all IDs, verify set equality)
   - Verify keyword search finds events containing the keyword

DELIVERABLE 3 — FAILURE SCENARIO TESTS

Write tests simulating infrastructure failures:

a) Database connection loss mid-insert:
   Use testcontainers to pause the PostgreSQL container during a BulkInsert call.
   Verify: ErrResourceExhausted or context error returned, no panic, connection 
   pool recovers after container resumes.

b) Poison pill payload:
   Send a batch where event[500] has an invalid timestamp and events[0-499] and 
   [501-999] are valid. Verify: entire batch is rejected (all-or-nothing semantics), 
   zero rows committed to database, 400 response with violation at "events[500].timestamp".

c) Context cancellation:
   Cancel the request context 50ms into a large batch insert.
   Verify: transaction is rolled back, no partial data committed, 
   connection is returned to pool (not leaked).

DELIVERABLE 4 — PERFORMANCE LOAD TEST (k6 script)

Write a complete k6 load test script (load_test.js) that:
- Ramps from 0 to 500 virtual users over 60 seconds
- Each VU sends POST /api/v1/logs with a realistic batch of 100 events
- Sustains 500 VU load for 5 minutes
- Ramps down over 30 seconds

Define k6 thresholds (fail the test if violated):
- http_req_duration: p95 < 200ms, p99 < 500ms
- http_req_failed: rate < 0.001 (less than 0.1% error rate)
- Custom metric: batch_events_per_second > 40000

Include realistic test data generation: randomized service names (10 options), 
realistic log messages, randomized attributes including user_id, request_id, 
http.status_code, and db.statement.
```

---

### Day 15 — React Frontend Scaffold

**Intent:** Establish a production-quality, type-safe React frontend architecture that can support a complex observability dashboard without accumulating technical debt.

```
ROLE: You are a Principal Frontend Engineer who has built production dashboard 
applications at the scale and complexity of Grafana or the Datadog web UI. 
You design frontend architectures that remain maintainable at 100k+ lines of code 
and where type safety eliminates entire categories of runtime errors.

PROJECT CONTEXT:
Scaffold the React 19 + TypeScript + Vite frontend for the OpenTrace observability 
console. This is not a prototype — it must be architected as a production application 
from the first commit. Every structural decision must be defensible for a codebase 
that will grow to dozens of components and hooks.

STACK:
- React 19 (use new concurrent features where appropriate)
- TypeScript 5.4+ with strict mode enabled (tsconfig strict: true)
- Vite 5 as build tool and dev server
- Tailwind CSS v4 for styling
- @tanstack/react-query v5 for server state management
- @tanstack/react-router v1 for type-safe routing
- Zod for runtime schema validation of API responses

DELIVERABLE 1 — PROJECT STRUCTURE

Produce a complete annotated directory tree:
web/
├── src/
│   ├── api/              # API client layer (one file per API resource)
│   ├── components/       
│   │   ├── ui/           # Primitive, reusable UI atoms (Button, Badge, Input, etc.)
│   │   └── features/     # Feature-specific composite components
│   │       ├── logs/
│   │       ├── metrics/
│   │       └── traces/
│   ├── hooks/            # Custom React hooks (data fetching, UI state)
│   ├── stores/           # Global client state (Zustand or React Context)
│   ├── types/            # TypeScript interfaces mirroring backend API schemas
│   ├── utils/            # Pure utility functions (formatters, parsers, validators)
│   ├── routes/           # TanStack Router route definitions
│   ├── lib/              # Third-party library configuration (queryClient, etc.)
│   └── main.tsx          # Application entry point
├── public/
├── index.html
├── vite.config.ts
├── tailwind.config.ts
└── tsconfig.json

Annotate every directory with its ownership rule (e.g., "components/ui must have zero 
business logic; components/features may import from api/ and hooks/ but not vice versa").

DELIVERABLE 2 — TYPE-SAFE API CLIENT LAYER

Implement the API client for the logs resource (src/api/logs.ts):

a) Define Zod schemas matching the backend API response exactly:
   - LogEventSchema: all fields with correct types
   - LogsQueryParamsSchema: all filter params with validation
   - LogsQueryResponseSchema: data array + next_cursor + query_time_ms

b) Define TypeScript types inferred from Zod schemas:
   type LogEvent = z.infer<typeof LogEventSchema>
   (never define types manually that duplicate schema — single source of truth)

c) Implement fetchLogs(params: LogsQueryParams): Promise<LogsQueryResponse>:
   - Use native Fetch API with AbortSignal support
   - Set base URL from import.meta.env.VITE_API_BASE_URL
   - Set default headers: Content-Type, Accept, X-Request-ID (generate UUID v4 per request)
   - On non-2xx response: parse RFC 7807 Problem Details JSON and throw typed ApiError
   - Parse and validate response with LogsQueryResponseSchema.parse() — throw on schema mismatch
   - Never catch and swallow errors — let them propagate to the query layer

d) Implement a base request utility function that all API modules use:
   apiFetch<T>(path: string, schema: ZodSchema<T>, options?: RequestInit): Promise<T>

DELIVERABLE 3 — BASE APPLICATION LAYOUT

Implement the shell layout components (src/components/features/shell/):

AppLayout.tsx:
- Responsive layout: collapsible sidebar (240px expanded, 60px collapsed) + main content area
- Sidebar contains: OpenTrace logo, nav items (Logs, Metrics, Traces, Alerts, Settings)
- Active route highlighting using TanStack Router's useMatch
- Keyboard shortcut: Cmd/Ctrl+B toggles sidebar collapse
- Persists collapsed state to localStorage

TopBar.tsx:
- Global time range selector (displayed persistently — it affects all dashboard views)
- Environment selector dropdown (production, staging, development)
- Search command palette trigger (Cmd/Ctrl+K)
- Connection status indicator (websocket/polling health)

Provide complete TypeScript code for both components.

DELIVERABLE 4 — VITE + TAILWIND CONFIGURATION

vite.config.ts:
- Path aliases: @ → src/, @api → src/api/, @components → src/components/
- Proxy: /api/* → http://localhost:8081 (local query-api dev proxy)
- Code splitting: manual chunks for vendor (react, react-dom), router, query, and app code

tailwind.config.ts:
- Extend theme with OpenTrace brand colors (dark slate palette for observability feel)
- Custom component classes for: badge variants (error=red, warn=amber, info=blue, debug=gray)
- Dark mode support (class strategy)

tsconfig.json:
- strict: true
- noUncheckedIndexedAccess: true (prevents common array/object access bugs)
- Path mappings matching vite.config.ts aliases
```

---

### Day 16 — Log Table Component

**Intent:** Build a virtualized, data-dense log table component that handles real-time streams of thousands of entries without performance degradation.

```
ROLE: You are a Senior Frontend UI/UX Developer who specializes in high-performance 
data-dense interfaces. You have built virtual list components for logging systems 
displaying millions of rows, and you treat DOM operations as a scarce resource.

PROJECT CONTEXT:
Build the core log table component for the OpenTrace dashboard. This component must 
handle real-time updates of thousands of log entries without layout jank, must 
visually communicate severity at a glance, and must allow operators to inspect 
full structured metadata without leaving the current view.

FILE: src/components/features/logs/LogTable.tsx

DELIVERABLE 1 — TYPE DEFINITIONS (src/types/logs.ts)

Define complete TypeScript interfaces:
interface LogEvent {
  id: string
  timestamp: string          // ISO-8601, display formatted in local timezone
  service_name: string
  severity: LogSeverity
  body: string               // truncate to 200 chars in table view
  trace_id?: string
  span_id?: string
  resource_attributes: Record<string, unknown>
  log_attributes: Record<string, unknown>
}

type LogSeverity = 'debug' | 'info' | 'warn' | 'error' | 'fatal'

interface LogTableProps {
  events: LogEvent[]
  isLoading: boolean
  hasNextPage: boolean
  onLoadMore: () => void
  onRowClick?: (event: LogEvent) => void
}

DELIVERABLE 2 — TABLE COMPONENT WITH VIRTUAL SCROLLING

Use @tanstack/react-virtual for row virtualization. Requirements:
- Virtualize rows when list exceeds 500 items (below that, render normally)
- Estimated row height: 44px (collapsed), dynamic height when expanded
- Overscan: 5 rows above and below the visible window
- Maintain scroll position when new rows are prepended (for live streaming mode)
- Column widths: Timestamp (180px fixed), Severity (80px fixed), 
  Service (200px, truncate), Message (flex, takes remaining space)

Severity badge visual design (Tailwind classes):
- fatal: bg-red-900 text-red-100 border border-red-700
- error: bg-red-600 text-white
- warn: bg-amber-500 text-amber-950
- info: bg-blue-600 text-white
- debug: bg-gray-600 text-gray-100

DELIVERABLE 3 — ROW EXPANSION PANEL

When a row is clicked, expand it inline (not a modal) to show:
- Full, untruncated log body
- resource_attributes rendered as a two-column key-value table
- log_attributes rendered as syntax-highlighted JSON (use a lightweight library 
  or implement simple JSON colorization with span tags)
- Copy-to-clipboard button for the full event as JSON
- Link-to-trace button (visible only if trace_id is present)
- Preserve expansion state when list updates (use Set<string> of expanded IDs in useState)

DELIVERABLE 4 — PERFORMANCE REQUIREMENTS

a) Memoization strategy:
   - Wrap LogTable with React.memo — define custom equality function that only 
     re-renders if events.length changes or the first/last event ID changes
   - Each LogRow must be its own memoized component
   - Severity badge must be a pure function component returning stable JSX

b) Empty, loading, and error states:
   - Loading: render 10 skeleton rows with shimmer animation (pure CSS, no external library)
   - Empty: centered illustration with "No logs found" message and filter reset button
   - Load more: intersection observer on a sentinel div at the bottom triggers onLoadMore

c) Column resize behavior:
   Each column header must be resizable via drag. Store widths in component state 
   (not global state). Min widths: Timestamp 140px, Severity 60px, Service 120px, Message 200px.

Provide the complete, compilable TypeScript + JSX implementation.
```

---

### Day 17 — Backend API Integration & Data Fetching Hooks

**Intent:** Connect the log table to the real backend API using production-grade data fetching patterns with caching, pagination, and cancellation.

```
ROLE: You are a Lead Full-Stack Engineer with deep expertise in React Query patterns, 
API integration, and asynchronous state management. You design data fetching layers 
that are resilient to network failures, avoid waterfall requests, and maintain 
cache coherence as filter parameters change.

PROJECT CONTEXT:
Integrate the OpenTrace log table component with the GET /api/v1/logs backend endpoint 
using @tanstack/react-query v5. The integration must support infinite scroll pagination, 
automatic cache invalidation when filters change, request cancellation on unmount or 
filter change, and structured error display.

DELIVERABLE 1 — QUERY KEY FACTORY (src/lib/queryKeys.ts)

Implement a type-safe query key factory for all log-related queries:
export const logKeys = {
  all: ['logs'] as const,
  lists: () => [...logKeys.all, 'list'] as const,
  list: (filters: LogsQueryParams) => [...logKeys.lists(), filters] as const,
}

This ensures: changing any filter parameter produces a new cache key and triggers 
a fresh fetch, while re-rendering with the same filters hits the cache.

DELIVERABLE 2 — INFINITE QUERY HOOK (src/hooks/useLogs.ts)

Implement function useLogs(params: LogsQueryParams):

Use useInfiniteQuery from @tanstack/react-query v5 with:
- queryKey: logKeys.list(params) — changes on every filter change
- queryFn: async ({ pageParam, signal }) => fetchLogs({ ...params, cursor: pageParam, signal })
  Pass the AbortSignal to fetchLogs to cancel in-flight requests on filter change
- initialPageParam: undefined (first page has no cursor)
- getNextPageParam: (lastPage) => lastPage.next_cursor ?? undefined
- staleTime: 30 * 1000 (30 seconds — balance freshness vs. network load)
- gcTime: 5 * 60 * 1000 (keep cache 5 minutes after component unmounts)
- refetchOnWindowFocus: false (log data changes rapidly; window focus refetch causes jarring reloads)

Return from the hook:
- events: LogEvent[] — flattened from all pages (useMemo over data.pages)
- isLoading, isFetchingNextPage, hasNextPage, error
- loadMore: () => void — calls fetchNextPage() if !isFetchingNextPage && hasNextPage

DELIVERABLE 3 — ERROR BOUNDARY AND ERROR DISPLAY

a) Implement LogsQueryErrorBoundary (class component) that catches render errors 
   and displays a recoverable error state with a "Retry" button.

b) Implement inline query error display in the LogTable integration:
   - If error is ApiError with status 400: display "Invalid filter parameters: {detail}" 
     with a "Clear filters" action button
   - If error is ApiError with status 503: display "Service temporarily unavailable. 
     Retrying in {countdown}s..." with automatic retry after 10 seconds
   - If error is network error (TypeError: Failed to fetch): display offline indicator
   - Never show raw error messages or stack traces to end users

DELIVERABLE 4 — TYPE CONTRACT ENFORCEMENT

Implement a compile-time contract that ensures frontend types stay in sync with backend:

a) In src/types/logs.ts, use Zod schemas as the single source of truth:
   export const LogEventSchema = z.object({ ... })
   export type LogEvent = z.infer<typeof LogEventSchema>

b) In the API client, validate every response:
   const validated = LogsQueryResponseSchema.parse(rawResponse)
   This ensures a backend schema change causes a visible runtime error in development 
   rather than silent undefined values in the UI.

c) Write a utility function formatLogTimestamp(isoString: string): string that:
   - Parses the ISO-8601 timestamp
   - Returns locale-aware format: "Jan 15, 14:32:01.847" 
   - Returns "Invalid date" gracefully if parsing fails (never throw)
   - Is memoized per isoString value (use a Map cache with max 10,000 entries)

DELIVERABLE 5 — LOGS PAGE COMPONENT (src/routes/logs.tsx)

Wire everything together in the Logs page:
- Render FilterBar (stub for now — implemented in Day 18)
- Render LogTable with data from useLogs(currentFilters)
- Show query statistics in the page header: "Showing {count} events | Query time: {ms}ms"
- Implement URL state sync: filter params are stored in URL search params so the 
  current view is bookmarkable and shareable
```

---

### Day 18 — Advanced Filter UI Components

**Intent:** Build a sophisticated, debounced, deep-linkable filter interface that coordinates multiple filter dimensions without degrading the user experience.

```
ROLE: You are a Senior Frontend UX Engineer who has designed complex search and 
filter interfaces for developer tools. You treat filter state as a first-class 
citizen — it must be bookmarkable, shareable, and resilient to invalid URL state.

PROJECT CONTEXT:
Build the multidimensional filter header for the OpenTrace log dashboard. The filter 
system must allow operators to rapidly narrow down log streams using service, severity, 
and time range dimensions simultaneously, with all state reflected in the URL for 
deep-link sharing.

FILE: src/components/features/logs/LogFilterBar.tsx

DELIVERABLE 1 — FILTER STATE MANAGEMENT (src/stores/logsFilterStore.ts)

Use Zustand to define the filter store:
interface LogsFilterState {
  serviceName: string | null
  levels: LogSeverity[]             // multi-select
  timeRange: TimeRange
  keyword: string
  setServiceName: (v: string | null) => void
  setLevels: (v: LogSeverity[]) => void
  setTimeRange: (v: TimeRange) => void
  setKeyword: (v: string) => void
  reset: () => void
}

type TimeRange = 
  | { type: 'relative'; duration: 5 | 15 | 60 | 240 | 1440 }  // minutes
  | { type: 'absolute'; start: string; end: string }           // ISO-8601

Implement URL sync: on every state change, write the filter state to URL search 
params using @tanstack/react-router's useNavigate. On component mount, initialize 
state from URL params with Zod validation (fall back to defaults for invalid URL state).

DELIVERABLE 2 — SERVICE NAME AUTOCOMPLETE (src/components/features/logs/ServiceSelect.tsx)

Requirements:
- Fetch unique service names from GET /api/v1/logs/services (implement this endpoint stub)
- Cache the service list for 60 seconds with react-query
- Render as a combobox: text input + dropdown list
- Filter options as user types (client-side filter on cached list, no per-keystroke API call)
- Keyboard navigation: Arrow keys move through options, Enter selects, Escape closes
- Show "All services" as the default/clear option
- Accessible: use ARIA combobox pattern with aria-expanded, aria-activedescendant

DELIVERABLE 3 — SEVERITY MULTI-SELECT (src/components/features/logs/SeverityFilter.tsx)

Render all 5 severity levels as toggle capsule buttons in a horizontal row.
Requirements:
- Each capsule shows: colored severity badge + count (e.g., "ERROR 1,247")
- Clicking toggles that severity in/out of the active filter set
- Shift+click selects a range
- "All" shortcut button clears all level filters
- Counts are fetched from a separate aggregate endpoint and update with the time range filter
- Animate count changes with a number flip transition

DELIVERABLE 4 — TIME RANGE PICKER (src/components/features/logs/TimeRangePicker.tsx)

Relative presets (displayed as pills): Last 5m, 15m, 1h, 4h, 24h
Absolute mode: two datetime-local inputs for start and end with validation:
- End must be after start
- Range must not exceed 7 days (matches API constraint)
- Invalid ranges show inline error, not a toast

Requirements:
- Default: Last 1 hour
- When switching from absolute back to relative, clear the absolute range
- Show the active range as a human-readable summary in the collapsed/closed state: 
  "Jan 15, 14:00 → 15:00" or "Last 1 hour"
- Keyboard accessible: Tab through inputs, Enter to apply

DELIVERABLE 5 — DEBOUNCING AND NETWORK OPTIMIZATION

Implement in the LogFilterBar coordinator component:
- Keyword input: debounce 400ms before updating filter state (avoid query-per-keystroke)
- Service name: no debounce (selected from dropdown, already a discrete action)
- Severity: no debounce (instant toggle)
- Time range: debounce 200ms for manual datetime input, immediate for preset selection
- Show a subtle loading spinner in the filter bar when a debounced value is pending 
  (visual feedback that input was received but query not yet fired)

Use useDeferredValue for the keyword field to prevent blocking the input render 
while react-query refetches.
```

---

### Day 19 — Auto-Refresh and Real-Time Polling System

**Intent:** Implement a resource-efficient polling system that delivers near-real-time log updates without degrading user experience or wasting infrastructure bandwidth.

```
ROLE: You are a Frontend Core Performance Engineer who specializes in real-time 
data synchronization. You design polling systems that are efficient enough to run 
on battery-powered laptops without noticeable CPU or network drain, while remaining 
responsive enough to be useful for live incident monitoring.

PROJECT CONTEXT:
Implement the auto-refresh system for the OpenTrace log dashboard. The system must 
fetch incremental log updates at configurable intervals, preserve the user's current 
scroll position and expanded row state, respect browser visibility, and avoid 
redundant network requests.

DELIVERABLE 1 — POLLING CONTROLLER HOOK (src/hooks/usePolling.ts)

Implement usePolling(callback: () => void, intervalMs: number, enabled: boolean):
- Uses setInterval internally — but correctly handles the stale closure problem 
  by using useRef for the callback (not re-creating the interval on every render)
- Clears interval on unmount (memory leak prevention)
- Clears and recreates interval when intervalMs changes
- Does nothing when enabled is false
- Returns: { isPolling: boolean }

DELIVERABLE 2 — PAGE VISIBILITY INTEGRATION (src/hooks/usePageVisibility.ts)

Implement usePageVisibility(): boolean using the Page Visibility API:
- Subscribes to document.addEventListener('visibilitychange', ...)
- Returns true when document.visibilityState === 'visible'
- Removes event listener on unmount
- Returns true on first render (assume visible until proven otherwise)

Integrate into the polling system: suspend all polling when the page is hidden.
Log visibility state changes at DEBUG level for observability of the feature.

DELIVERABLE 3 — AUTO-REFRESH CONFIGURATION UI (src/components/features/logs/RefreshControl.tsx)

Render a compact control showing:
- Current refresh interval selector: Off | 5s | 10s | 30s | 1m
- Visual "last refreshed X seconds ago" counter that increments in real-time
- Pause/resume button (independent of interval selection — "paused" means 
  "keep the interval setting but don't execute it right now")
- Pulsing green dot when polling is active, gray when paused
- Persist selected interval to localStorage (restore on page load)

DELIVERABLE 4 — INCREMENTAL UPDATE LOGIC (src/hooks/useIncrementalLogs.ts)

On each poll tick, fetch only new logs since the last known newest timestamp:
- Track newestTimestamp: string in a ref (updated after each successful fetch)
- On poll: call fetchLogs({ ...currentFilters, startTime: newestTimestamp, endTime: now() })
- Prepend new results to the existing list (do NOT replace — preserve scroll context)
- Deduplicate by event id before prepending (handle edge case where same event 
  appears in both poll results if timestamp is on a boundary)
- Cap total in-memory events to 10,000 (drop oldest entries when limit exceeded)
- Show a "N new events" banner when new events arrive while user is scrolled down; 
  clicking the banner scrolls to top

DELIVERABLE 5 — PERFORMANCE OPTIMIZATIONS

a) Memoize the event list transformation:
   const flatEvents = useMemo(() => 
     pages.flatMap(page => page.data),
     [pages]  // only recompute when pages reference changes
   )

b) Prevent full list re-render on incremental prepend:
   Use a stable key strategy: event id (not index) as React key.
   New events prepended at index 0 should not cause existing rows to re-render.
   Verify with React DevTools Profiler: existing rows should show 0 renders on update.

c) Implement stale-while-revalidate display logic:
   Show a subtle "updating..." indicator on the last-refreshed timestamp 
   while a poll request is in-flight, rather than blanking the data.
   Never show a full-page loading spinner during background polling.
```

---

### Day 20 — Docker Full Stack Integration

**Intent:** Consolidate the complete MVP into a single, hardened Docker Compose configuration with strict security boundaries and environment-based configuration.

```
ROLE: You are a Principal DevSecOps Engineer who designs container environments 
that mirror production security posture as closely as possible in local development. 
You treat "it works in docker compose" as a prerequisite for "it works in Kubernetes".

PROJECT CONTEXT:
Produce the final, consolidated Docker Compose configuration for the complete 
OpenTrace log ingestion MVP. All four services must be orchestrated in a single 
compose file with strict network isolation, proper health checking, and 
secure environment-based configuration.

SERVICES:
- collector-service (Go): Receives SDK telemetry, port 8080 (internal only)
- query-api (Go): Serves dashboard queries, port 8081 (proxied via UI nginx)
- opentrace-ui (React/Nginx): Serves frontend, port 3000 (external)
- postgres (PostgreSQL 16): Log storage, port 5432 (internal only)

DELIVERABLE 1 — docker-compose.yml (complete, production-grade)

Network topology:
- backend: Internal bridge network — postgres, collector-service, query-api only
- frontend: External bridge network — opentrace-ui, query-api only
- Collector-service on backend only (not exposed to frontend network or host)
- Query-api on both networks (bridges internal data and external UI)
- Postgres on backend only (never directly accessible from host in production mode)

For each service include:
- Exact image version (pinned)
- environment: section reading from .env file
- depends_on: with condition: service_healthy
- Health check with start_period: 30s, interval: 10s, timeout: 5s, retries: 5
- deploy.resources.limits: memory and cpus
- restart: unless-stopped

postgres health check: pg_isready -U $${POSTGRES_USER} -d $${POSTGRES_DB}
collector health check: wget -qO- http://localhost:8080/healthz || exit 1
query-api health check: wget -qO- http://localhost:8081/healthz || exit 1
opentrace-ui health check: wget -qO- http://localhost:3000/health || exit 1

Nginx configuration for opentrace-ui:
- Reverse proxy /api/* to http://query-api:8081/api/* 
  (so the UI never needs to know the API host)
- SPA routing: try_files $uri $uri/ /index.html
- gzip compression for all text/* and application/json responses
- Security headers: X-Frame-Options, X-Content-Type-Options, 
  Content-Security-Policy (strict), Referrer-Policy

DELIVERABLE 2 — .env.example (complete)

Provide a .env.example with all variables used across all services, grouped by service:
# === POSTGRES ===
POSTGRES_USER=opentrace
POSTGRES_PASSWORD=changeme_in_production
POSTGRES_DB=opentrace
POSTGRES_HOST=postgres
POSTGRES_PORT=5432
POSTGRES_MAX_CONNS=20

# === COLLECTOR SERVICE ===
COLLECTOR_PORT=8080
COLLECTOR_LOG_LEVEL=info
COLLECTOR_MAX_BATCH_SIZE=1000
COLLECTOR_MAX_PAYLOAD_BYTES=5242880
COLLECTOR_DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${POSTGRES_DB}

# === QUERY API ===
QUERY_API_PORT=8081
QUERY_API_LOG_LEVEL=info
QUERY_API_DATABASE_URL=... (same pattern)
QUERY_API_DEFAULT_PAGE_SIZE=100

# === UI ===
UI_PORT=3000
VITE_API_BASE_URL=  (empty — nginx proxies /api/* internally)

DELIVERABLE 3 — Database Initialization

Create infra/postgres/init/001_schema.sql that is mounted as a volume at 
/docker-entrypoint-initdb.d/001_schema.sql in the postgres container.
This file must contain the complete idempotent schema from Day 9:
- log_severity enum type creation
- logs partitioned table creation
- All indexes
- pg_cron schedule setup (if available) or commented instructions

DELIVERABLE 4 — Operational Makefile Targets

Extend the Makefile with targets:
- make up: docker compose up -d --build
- make down: docker compose down
- make reset: docker compose down -v (destroys all volumes — with confirmation prompt)
- make logs service=collector-service: docker compose logs -f <service>
- make health: curl all health endpoints and print PASS/FAIL per service
- make seed: run a Go script that sends 1,000 sample log events to the collector
- make psql: docker compose exec postgres psql -U $POSTGRES_USER -d $POSTGRES_DB
```

---

### Day 21 — Phase 1 Pre-Production Readiness Review

**Intent:** Execute a comprehensive code review across the entire Phase 1 implementation to identify and remediate production risks before advancing to Phase 2.

```
ROLE: You are a Distinguished Principal Engineer conducting a critical pre-production 
readiness review. You have shipped observability platforms to Fortune 500 companies 
and you have the scar tissue to know exactly where systems fail at 3am on a Sunday. 
Your job is to find every landmine before it explodes in production.

REVIEW SCOPE:
The complete Phase 1 (Log Ingestion MVP) implementation of OpenTrace, comprising:
- Go collector-service (HTTP handler, validation, PostgreSQL persistence)
- Go query-api (query endpoint, keyset pagination, dynamic query builder)
- PostgreSQL schema (partitioned logs table, indexes, partitioning automation)
- React frontend (log table, filter bar, API integration, auto-refresh)
- Docker Compose stack (multi-service, networked, health-checked)

[PASTE ALL IMPLEMENTATION CODE AND CONFIGURATION FILES FROM DAYS 8-20 HERE]

REVIEW CATEGORY 1 — RACE CONDITIONS & MEMORY LEAKS (Go)

Audit every goroutine, channel, and deferred resource closure in the Go services:
- Identify any goroutines that can leak on request cancellation or server shutdown
- Verify every http.Response.Body is closed in all code paths (not just happy path)
- Verify pgxpool is closed on graceful shutdown and not before all queries complete
- Identify any shared mutable state accessed without synchronization
- Check for context propagation gaps: any database call not receiving the request context
- Verify http.MaxBytesReader is applied before any json.Decoder is created

For each finding: show the exact code location, the failure scenario, and the fix.

REVIEW CATEGORY 2 — SQL QUERY CORRECTNESS & PERFORMANCE

Audit the dynamic query builder and persistence layer:
- Verify zero SQL injection vectors: every user-supplied value must be a parameter
- Verify the keyset cursor query is logically correct for the (timestamp, id) ordering
- Verify the query correctly handles the case where multiple events share the same timestamp
- Audit index usage: verify the WHERE clause column order matches composite index column order
- Verify CopyFrom column list exactly matches the table DDL column order
- Identify any query that could trigger a sequential scan under realistic filter combinations

REVIEW CATEGORY 3 — FRONTEND PERFORMANCE & MEMORY LEAKS (React)

Audit the React component tree and data fetching layer:
- Identify any useEffect cleanup functions that are missing (event listeners, timers, subscriptions)
- Verify AbortController is correctly wiring to fetch cancellation on filter change
- Identify any re-render paths where the entire log list re-renders on a single new event
- Verify virtualization is correctly applied and row keys are stable (not index-based)
- Identify any large objects captured in closures inside useCallback/useMemo dependencies
- Verify the polling system correctly suspends on page visibility change

REVIEW CATEGORY 4 — INFRASTRUCTURE & SECURITY

Audit the Docker Compose and operational configuration:
- Verify the database is not exposed on the host network
- Verify all container images run as non-root users
- Identify any hardcoded secrets (passwords, tokens) in any committed file
- Verify health checks correctly gate service startup order
- Identify any missing resource limits that could cause container OOM on the host
- Verify nginx CSP header does not inadvertently block the API proxy calls

FINAL DELIVERABLE — REMEDIATION MATRIX

Produce a prioritized remediation table:
| Severity | Category | File/Component | Issue Description | Recommended Fix | Estimated Effort |
|----------|----------|----------------|-------------------|-----------------|-----------------|
| CRITICAL | ... | ... | ... | ... | S/M/L |
| WARNING | ... | ... | ... | ... | S/M/L |
| OPTIMIZATION | ... | ... | ... | ... | S/M/L |

CRITICAL = must fix before Phase 2 development begins
WARNING = must fix before any external users or load testing
OPTIMIZATION = fix before 10k events/sec target

End with a Phase 2 readiness score (0-100) and a list of blocking issues that 
must be resolved before the SDK development phase begins.
```

---

## Phase 2 — SDK Development

---

### Day 22 — Go SDK Design Specification

**Intent:** Define the complete public API surface and internal architecture for the OpenTrace Go SDK before writing any implementation code.

```
ROLE: You are a Lead Developer Tools Architect who has designed and maintained 
widely-adopted Go instrumentation libraries (think zerolog, zap, OpenTelemetry Go SDK). 
You understand that a logging SDK is embedded in production critical paths — 
any allocations, latency, or blocking behavior you introduce will degrade the 
host application directly.

PROJECT CONTEXT:
Design the complete API specification and architectural blueprint for the 
OpenTrace Go logging SDK (module: github.com/opentrace/opentrace-go). 
This SDK will be imported by enterprise Go applications and must:
- Add < 5 microseconds overhead per log call on the hot path
- Produce zero heap allocations per log call at steady state
- Never block the calling goroutine for network I/O
- Be safe for concurrent use from any number of goroutines

DELIVERABLE 1 — PUBLIC API SURFACE SPECIFICATION

Define the complete public API in Go interface notation (not implementation):

Package-level initialization:
  opentrace.New(opts ...Option) (*Logger, error)
  opentrace.NewNop() *Logger  // no-op logger for testing

Logger methods (zero-allocation hot path):
  logger.Debug(msg string, fields ...Field) 
  logger.Info(msg string, fields ...Field)
  logger.Warn(msg string, fields ...Field)
  logger.Error(msg string, fields ...Field)
  logger.Fatal(msg string, fields ...Field)  // logs then calls os.Exit(1)

  logger.With(fields ...Field) *Logger  // returns a child logger with pre-set fields
  logger.WithContext(ctx context.Context) *Logger  // extracts trace_id, span_id from ctx

Structured field constructors (must produce zero allocations for scalar types):
  opentrace.String(key, value string) Field
  opentrace.Int(key string, value int) Field
  opentrace.Int64(key string, value int64) Field
  opentrace.Float64(key string, value float64) Field
  opentrace.Bool(key string, value bool) Field
  opentrace.Error(err error) Field  // key: "error", value: err.Error()
  opentrace.Duration(key string, value time.Duration) Field
  opentrace.Any(key string, value interface{}) Field  // falls back to fmt.Sprintf

Lifecycle methods:
  logger.Shutdown(ctx context.Context) error  // flush buffers, wait for delivery

DELIVERABLE 2 — INTERNAL ARCHITECTURE DESIGN

Write a detailed technical design document (600+ words) covering:

a) SEPARATION OF CONCERNS
   The SDK must separate three layers with zero shared mutable state:
   Layer 1 — Application API (logger.Info, Field constructors): called on hot path, 
             must be allocation-free for all non-Any field types
   Layer 2 — Internal Buffer (ring buffer or channel): accepts events from Layer 1 
             without blocking; drops events (with a dropped counter) if full
   Layer 3 — Background Exporter (goroutine): drains the buffer, batches events, 
             sends HTTP payloads, handles retries

b) ZERO-ALLOCATION HOT PATH DESIGN
   Explain the techniques required to achieve zero allocations:
   - Why Field must be a value type (struct with a union-like representation) 
     rather than an interface
   - How sync.Pool is used to reuse log event structs between calls
   - Why reflection (encoding/json default behavior) is forbidden on the hot path
   - How to pre-allocate and reuse the fields array

c) RING BUFFER vs CHANNEL
   Compare using a circular ring buffer (array-based, lock-free with atomics) 
   vs. a buffered Go channel as the internal queue:
   - Throughput benchmark estimates (ops/sec, allocations/op)
   - Behavior when full (drop oldest vs drop newest vs block)
   - Implementation complexity trade-off
   Recommend one approach for the MVP and document the threshold at which 
   the other becomes necessary.

d) CONTEXT PROPAGATION
   How the SDK extracts OpenTelemetry trace_id and span_id from a context.Context 
   without importing the full OpenTelemetry SDK (use interface assertion on known keys).

DELIVERABLE 3 — ALLOCATION BUDGET

Define the SDK's allocation budget as a formal specification:
- logger.Info("message", opentrace.String("k", "v")): 0 allocs/op, 0 bytes/op
- logger.Info("message", opentrace.Any("k", someStruct)): 1 alloc/op (the interface boxing)
- logger.With(fields...): 1 alloc/op (new Logger child struct)
- Batch flush (100 events): N allocs where N is deterministic and bounded

These budgets must be enforced via benchmark tests: 
go test -bench=. -benchmem must show ≤ these numbers.
```

---

### Day 23 — SDK Core Implementation

**Intent:** Implement the complete, production-grade SDK core following the Day 22 specification exactly, with benchmark tests validating the allocation budget.

```
ROLE: You are a Staff Go Systems Engineer with expertise in low-level Go performance 
optimization. You have contributed to high-performance Go libraries and you use 
go tool pprof and go test -benchmem as primary development feedback loops, 
not afterthoughts.

PROJECT CONTEXT:
Implement the core of the OpenTrace Go SDK (sdk/go/) based on the Day 22 specification. 
This is the implementation phase — produce complete, compilable Go code that satisfies 
the public API surface, meets the allocation budget, and passes all unit tests.

Go module path: github.com/opentrace/opentrace-go
Minimum Go version: 1.22

FILE STRUCTURE TO PRODUCE:
sdk/go/
├── logger.go          — Logger struct and public methods
├── field.go           — Field type and all constructor functions  
├── level.go           — Level type and constants
├── event.go           — LogEvent struct (internal wire type)
├── pool.go            — sync.Pool configuration for event recycling
├── buffer.go          — Internal event buffer (channel or ring buffer)
├── config.go          — Config struct (stub — full implementation Day 27)
├── nop.go             — No-op logger implementation
└── logger_test.go     — Unit tests and benchmarks

STEP 1 — FIELD TYPE IMPLEMENTATION (field.go)

Field must be a concrete struct (not an interface) to avoid heap allocation:
type Field struct {
    Key       string
    Type      fieldType  // enum: typeString, typeInt64, typeFloat64, typeBool, typeError, typeAny
    StringVal string
    Int64Val  int64
    Float64Val float64
    BoolVal   bool
    Interface interface{}  // only set for typeAny and typeError
}

Implement all constructor functions. Verify via benchmark:
func BenchmarkStringField(b *testing.B) {
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        _ = String("key", "value")  // must report 0 allocs/op
    }
}

STEP 2 — LOGGER CORE (logger.go)

type Logger struct {
    level    Level
    fields   []Field      // pre-set fields from With()
    buffer   *eventBuffer // shared across all child loggers
    resource resourceInfo // host.name, service.name, process.pid — captured at init
    dropped  atomic.Int64 // count of dropped events when buffer is full
}

func (l *Logger) log(level Level, msg string, fields []Field):
  - Check level >= l.level first (fast path: return early if below minimum level)
  - Acquire a LogEvent from sync.Pool
  - Populate event fields without heap allocation
  - Attempt non-blocking send to l.buffer
  - If buffer full: increment l.dropped atomic counter, return the event to pool
  - This method must execute in < 500ns under zero contention

STEP 3 — SYNC.POOL CONFIGURATION (pool.go)

var eventPool = sync.Pool{
    New: func() interface{} {
        return &LogEvent{
            Fields: make([]Field, 0, 16),  // pre-allocate fields slice capacity
        }
    },
}

Implement acquire() *LogEvent and release(e *LogEvent):
- release must reset all fields before returning to pool (prevent data leaks between calls)
- release must reset the Fields slice to zero length (but keep the backing array)
- Never release an event that has been sent to the buffer (use-after-free prevention)

STEP 4 — RESOURCE CAPTURE (internal/resource.go)

Implement func captureResource(cfg *Config) resourceInfo that captures at startup:
- host.name: os.Hostname()
- process.pid: os.Getpid()
- service.name: from Config or OPENTRACE_SERVICE_NAME env var
- service.version: from Config or build-time ldflags injection
These are captured once at init — never per log call.

STEP 5 — UNIT TESTS AND BENCHMARKS (logger_test.go)

Unit tests:
- TestLoggerLevelFiltering: events below minimum level must not reach the buffer
- TestLoggerWith: child logger must include parent fields plus its own
- TestNopLogger: all methods return without error, buffer remains empty
- TestDroppedCounter: fill the buffer completely, verify dropped counter increments

Benchmarks (must all pass with 0 allocs/op):
- BenchmarkLoggerInfo_NoFields
- BenchmarkLoggerInfo_ThreeStringFields
- BenchmarkLoggerInfo_MixedFields
- BenchmarkLoggerWith (1 alloc expected for the new Logger struct)
```

---

### Day 24 — Internal Batching Engine

**Intent:** Implement the dual-trigger (time + size) batch accumulation system with mathematical justification for the chosen buffer parameters.

```
ROLE: You are an Expert Systems Engineer specializing in high-velocity data pipeline 
internals. You have implemented batching systems for log shippers, metrics collectors, 
and event streaming platforms. You understand that batching parameters are performance 
contracts, not configuration knobs — they must be derived from mathematical first principles.

PROJECT CONTEXT:
Implement the internal memory batching engine for the OpenTrace Go SDK. This engine 
sits between the lock-free event buffer (Day 23) and the network exporter (Day 25). 
It accumulates events and flushes them in structured batches to minimize HTTP 
round-trips without introducing unbounded latency.

FILE: sdk/go/internal/batcher/batcher.go

MATHEMATICAL ANALYSIS — produce this BEFORE the implementation:

Given:
- Target ingest rate: R events/second per SDK instance
- Maximum acceptable delivery latency: L seconds
- HTTP transmission overhead per request: O ms
- Collector throughput capacity: C requests/second

Derive:
- Optimal batch size S = R × L (events that accumulate in one latency window)
- Network efficiency gain = S / (S + O×R/1000) (fraction of time sending data vs overhead)
- Memory footprint at steady state ≈ S × avg_event_size_bytes

Produce a table showing batch size recommendations for three SDK deployment profiles:
| Profile | Target Rate | Max Latency | Recommended Batch Size | Flush Interval |
|---------|------------|-------------|----------------------|----------------|
| Low-traffic service | 100 eps | 5s | ... | ... |
| Standard web service | 1,000 eps | 2s | ... | ... |
| High-frequency service | 10,000 eps | 500ms | ... | ... |

IMPLEMENTATION:

type Batcher struct {
    maxSize     int           // flush when batch reaches this many events
    maxBytes    int           // flush when batch reaches this many bytes
    maxInterval time.Duration // flush when this duration has elapsed since last flush
    // internal state...
}

STEP 1 — BATCH ACCUMULATION
- Internal slice pre-allocated to maxSize capacity (avoid re-allocation on append)
- Thread-safe: all public methods must be safe for concurrent callers
- AddEvent(event *LogEvent) bool: adds event to current batch, returns true if flush triggered

STEP 2 — DUAL TRIGGER FLUSH LOGIC
Flush is triggered when ANY of these conditions is true (whichever comes first):
1. len(currentBatch) >= maxSize
2. currentBatchBytes >= maxBytes (sum of serialized event sizes)
3. time.Since(lastFlushTime) >= maxInterval (evaluated on a background timer tick)

When flush is triggered:
- Swap currentBatch with a fresh empty slice (atomic swap to minimize lock hold time)
- Send the full batch to a flush channel (non-blocking — if channel is full, log warning and drop)
- Reset lastFlushTime

STEP 3 — MEMORY FOOTPRINT ANALYSIS

Write a 300-word analysis covering:
- Bounded vs. unbounded queue design: why using a fixed-capacity batch with drop-on-full 
  is preferable to an unbounded queue for protecting the host application from OOM
- Memory footprint formula: maxSize × avg_event_bytes × 2 (double buffer during flush swap)
- What happens to the in-flight batch if the application crashes before flush: 
  document this as an explicit data loss window (not a bug — a design decision)
- How to choose maxSize to achieve < 0.1% data loss under normal operation 
  (derive from target flush interval and crash probability model)

STEP 4 — UNIT TESTS (batcher_test.go)
- TestBatchFlushOnSize: add exactly maxSize events, verify flush triggered
- TestBatchFlushOnBytes: add events until byte limit exceeded, verify flush triggered
- TestBatchFlushOnTimer: add 1 event, wait maxInterval + 10ms, verify flush triggered
- TestConcurrentAdd: 100 goroutines adding simultaneously, verify no events duplicated or lost
- TestFlushChannelFull: fill the flush output channel, verify AddEvent returns false gracefully
```

---

### Day 25 — Background Exporter Worker

**Intent:** Implement the goroutine-based background worker that drains batches from the batcher and transmits them to the collector via HTTP.

```
ROLE: You are an Expert Go Concurrency Engineer with deep knowledge of goroutine 
lifecycle management, channel communication patterns, and context propagation. 
You have debugged goroutine leaks in production systems and you treat every 
goroutine as a resource that must be explicitly owned and terminated.

PROJECT CONTEXT:
Implement the background exporter worker for the OpenTrace Go SDK. This worker 
runs in a dedicated goroutine, drains batches from the batcher's flush channel, 
serializes them to JSON, and transmits them to the OpenTrace collector-service 
via HTTP POST. It must be stoppable via context cancellation and must drain 
any pending batches before exiting.

FILE: sdk/go/internal/exporter/http_exporter.go

STEP 1 — EXPORTER INTERFACE

type Exporter interface {
    Start(ctx context.Context) error
    Export(ctx context.Context, batch []*LogEvent) error
    Shutdown(ctx context.Context) error
}

Define HTTPExporter implementing this interface.

STEP 2 — WORKER GOROUTINE LOOP

func (e *HTTPExporter) Start(ctx context.Context) error:
- Spawn exactly one background goroutine (the "drain loop")
- The drain loop must use select with two cases:
    case batch := <-e.flushCh:  // process a batch from the batcher
        e.Export(ctx, batch)
    case <-ctx.Done():          // shutdown signal received
        e.drainAndClose()
        return
- NEVER use a for { batch := <-ch } pattern without a ctx.Done() case — 
  this goroutine will leak if the context is never cancelled
- Track the goroutine with a sync.WaitGroup so Shutdown() can wait for it

STEP 3 — BATCH SERIALIZATION

Implement serialize(batch []*LogEvent) ([]byte, error):
- Use encoding/json Marshal — note this is NOT on the hot path (background goroutine only)
- Re-use a bytes.Buffer from sync.Pool to avoid allocations per batch
- Apply gzip compression if batch size > compressionThresholdBytes (default: 1KB)
- Set appropriate Content-Encoding header based on whether compression was applied

STEP 4 — HTTP TRANSMISSION

Implement Export(ctx context.Context, batch []*LogEvent) error:
- Use a shared *http.Client with explicit timeouts (not the default http.Client):
  * Timeout: 10s total request timeout
  * Transport: MaxIdleConns 10, IdleConnTimeout 90s, DisableCompression true 
    (we handle compression manually)
- POST to e.collectorEndpoint + "/api/v1/logs"
- Set headers: Content-Type: application/json, Content-Encoding: gzip (if compressed),
  X-SDK-Version: <version>, User-Agent: opentrace-go/<version>
- Read and discard response body (always) before closing (prevents connection pool leaks)
- On non-2xx response: classify error and return for retry layer (Day 26) to handle

STEP 5 — GRACEFUL DRAIN ON SHUTDOWN

func (e *HTTPExporter) drainAndClose():
- After ctx.Done() fires, drain ALL remaining items from flushCh before returning
  (use a non-blocking select loop: for { select { case b := <-ch: export(b); default: return } })
- This ensures zero data loss for events already in the flush channel at shutdown time
- Set a maximum drain timeout of 5 seconds (caller provides context with deadline)
- Log the count of batches drained at INFO level

STEP 6 — RACE CONDITION DOCUMENTATION

Identify and document the following specific race conditions that must be prevented:
a) Double-close race: Shutdown() called twice — protect with sync.Once
b) Export called after Shutdown: return ErrShutdown immediately
c) flushCh send and drain racing at shutdown boundary: 
   explain how the WaitGroup and channel drain order prevents loss
d) Context cancellation during active Export: 
   the in-flight HTTP request respects the context — explain the behavior

STEP 7 — TESTS
- TestExporterStartStop: start, send 10 batches, shutdown, verify all batches received by mock server
- TestExporterShutdownDrain: queue 5 batches then call shutdown, verify all 5 are transmitted
- TestExporterContextCancellation: cancel context mid-export, verify clean exit
- TestExporterConcurrentExport: 10 goroutines calling Export simultaneously, verify no data races 
  (run with go test -race)
```

---

### Day 26 — Retry and Backoff System

**Intent:** Implement a mathematically correct exponential backoff system with jitter, circuit breaking, and explicit data loss boundaries.

```
ROLE: You are a Principal Distributed Systems Engineer specializing in fault-tolerant 
network communication. You have designed retry systems for payment processors, 
message queues, and telemetry pipelines where the cost of both over-retrying 
and under-retrying is measured in dollars and data loss respectively.

PROJECT CONTEXT:
Implement the retry and circuit breaker layer for the OpenTrace SDK HTTP exporter. 
This layer wraps the Export() method and handles transient collector failures 
gracefully without blocking the host application or causing OOM via unbounded retry queues.

FILE: sdk/go/internal/retry/retry.go

STEP 1 — BACKOFF ALGORITHM IMPLEMENTATION

Implement func ExponentialBackoff(attempt int, cfg BackoffConfig) time.Duration:

Formula: T_wait = min(T_max, T_base × 2^attempt) + jitter
Where: jitter = random value in [0, T_jitter_max]

BackoffConfig:
type BackoffConfig struct {
    BaseDelay  time.Duration  // default: 100ms
    MaxDelay   time.Duration  // default: 30s
    MaxJitter  time.Duration  // default: 1s
    MaxAttempts int           // default: 5
}

Produce a table in comments showing wait times for attempts 0-5 with default config:
| Attempt | Base Wait | With Max Jitter | Cumulative Max Wait |
|---------|-----------|-----------------|---------------------|
| 0       | 100ms     | 1.1s            | 1.1s                |
| 1       | 200ms     | 1.2s            | 2.3s                |
| ...     | ...       | ...             | ...                 |

The jitter component is critical — explain in a comment block why deterministic 
backoff without jitter causes thundering herd: if 1,000 SDK instances all fail 
simultaneously and back off to the same interval, they all retry at the same moment, 
potentially overwhelming the recovering collector.

STEP 2 — RETRY WRAPPER

Implement func WithRetry(ctx context.Context, cfg BackoffConfig, fn func(context.Context) error) error:
- Execute fn(ctx)
- On success: return nil immediately
- On retryable error (network error, 429, 503): wait ExponentialBackoff(attempt, cfg), retry
- On non-retryable error (400, 401, 403, 413): return immediately — do not retry
- On max attempts exceeded: return ErrMaxRetriesExceeded wrapping the last error
- On ctx.Done(): return ctx.Err() immediately — never retry a cancelled context
- Log each retry attempt at WARN level with: attempt number, error, next wait duration

Define the retryable error classification:
func isRetryable(err error) bool — check for:
- *url.Error (network-level failures)
- HTTP status 429 (rate limited) — read Retry-After header if present
- HTTP status 500, 502, 503, 504 (server errors)
- io.EOF, io.ErrUnexpectedEOF (connection reset by collector)
NOT retryable: 400, 401, 403, 404, 413 (client errors — retrying won't help)

STEP 3 — CIRCUIT BREAKER

Implement a simple three-state circuit breaker (Closed → Open → Half-Open):

type CircuitBreaker struct {
    maxFailures    int           // consecutive failures before opening: default 5
    resetTimeout   time.Duration // time in Open state before trying Half-Open: default 60s
    // internal state...
}

State transitions:
- CLOSED → OPEN: after maxFailures consecutive failures
- OPEN → HALF-OPEN: after resetTimeout elapses
- HALF-OPEN → CLOSED: next call succeeds
- HALF-OPEN → OPEN: next call fails

When in OPEN state: return ErrCircuitOpen immediately without calling Export() — 
this is the critical data loss / host protection boundary.

STEP 4 — DATA LOSS POLICY DOCUMENTATION

Write an explicit policy document (as Go doc comments) covering:
a) When the SDK drops telemetry data (and why this is correct behavior):
   1. Internal event buffer is full (batcher): newest event dropped
   2. Flush channel is full (exporter): entire batch dropped  
   3. Circuit breaker is OPEN: entire batch dropped with increment to dropped counter
   4. Max retries exceeded: batch dropped after logging the error

b) How to monitor data loss from the host application:
   logger.DroppedCount() int64  — expose the dropped counter

c) Why blocking is worse than dropping:
   Quantify the trade-off: a 1-second block in a logger.Info() call in a 
   web server handler directly increases request latency p99 by 1 second — 
   this is worse than losing telemetry data.

STEP 5 — TESTS
- TestBackoffTiming: verify wait times match the formula for attempts 0-4
- TestJitterRange: verify 1000 samples of jitter are within [0, MaxJitter]
- TestRetryOnTransientError: mock server returns 503 twice then 202; verify 3 total attempts
- TestNoRetryOnClientError: mock server returns 400; verify exactly 1 attempt
- TestCircuitBreakerOpens: trigger 5 consecutive failures; verify circuit opens
- TestCircuitBreakerResets: open circuit, wait resetTimeout, verify half-open succeeds
```

---

### Day 27 — SDK Configuration System

**Intent:** Implement a type-safe, environment-aware configuration system using the functional options pattern with startup-time validation.

```
ROLE: You are a Lead Developer Experience (DX) Engineer who has designed public APIs 
for developer tools used by thousands of teams. You understand that configuration 
ergonomics directly affect SDK adoption — a confusing config system leads to 
misconfigured deployments and support tickets.

PROJECT CONTEXT:
Implement the complete configuration system for the OpenTrace Go SDK. The system 
must support three configuration sources in priority order:
1. Explicit programmatic options (highest priority)
2. Environment variables (fallback)
3. Hardcoded defaults (lowest priority)

FILE: sdk/go/config.go and sdk/go/options.go

DELIVERABLE 1 — CONFIG STRUCT

type Config struct {
    // Required
    CollectorEndpoint string        // e.g., "https://collector.example.com"
    ServiceName       string        // e.g., "payment-service"
    
    // Optional with defaults
    ServiceVersion    string        // default: "unknown"
    Environment       string        // default: "production"
    MinLevel          Level         // default: LevelInfo
    BatchSize         int           // default: 500, max: 5000
    BatchInterval     time.Duration // default: 2s, max: 60s
    MaxBatchBytes     int           // default: 1MB, max: 5MB
    BufferSize        int           // default: 10000 (ring buffer capacity)
    HTTPTimeout       time.Duration // default: 10s
    MaxRetries        int           // default: 5
    Headers           map[string]string // custom headers to add to all requests (e.g., auth)
    
    // Advanced
    CompressionEnabled bool         // default: true
    Debug              bool         // default: false (emit SDK-internal logs to stderr)
}

DELIVERABLE 2 — FUNCTIONAL OPTIONS PATTERN

type Option func(*Config)

Implement an Option constructor for every Config field:
func WithCollectorEndpoint(endpoint string) Option
func WithServiceName(name string) Option
func WithServiceVersion(version string) Option
func WithEnvironment(env string) Option
func WithMinLevel(level Level) Option
func WithBatchSize(size int) Option
func WithBatchInterval(d time.Duration) Option
func WithHeader(key, value string) Option  // can be called multiple times
func WithDebug(enabled bool) Option
// etc.

Usage must look like:
logger, err := opentrace.New(
    opentrace.WithCollectorEndpoint("https://collector.acme.com"),
    opentrace.WithServiceName("checkout-service"),
    opentrace.WithBatchSize(250),
    opentrace.WithHeader("X-API-Key", os.Getenv("OPENTRACE_API_KEY")),
)

DELIVERABLE 3 — ENVIRONMENT VARIABLE FALLBACK

Implement func loadFromEnv(cfg *Config):
Apply env vars to any Config field that is still at its zero value after options are applied.

Environment variable mapping (document all in a table):
| Env Var | Config Field | Type | Example |
|---------|-------------|------|---------|
| OPENTRACE_COLLECTOR_ENDPOINT | CollectorEndpoint | string | https://... |
| OPENTRACE_SERVICE_NAME | ServiceName | string | payment-service |
| OPENTRACE_SERVICE_VERSION | ServiceVersion | string | 1.4.2 |
| OPENTRACE_ENVIRONMENT | Environment | string | production |
| OPENTRACE_MIN_LEVEL | MinLevel | string | warn |
| OPENTRACE_BATCH_SIZE | BatchSize | int | 500 |
| OPENTRACE_BATCH_INTERVAL_MS | BatchInterval | int (ms) | 2000 |
| OPENTRACE_BUFFER_SIZE | BufferSize | int | 10000 |
| OPENTRACE_HTTP_TIMEOUT_MS | HTTPTimeout | int (ms) | 10000 |
| OPENTRACE_DEBUG | Debug | bool | true/false |

DELIVERABLE 4 — STARTUP VALIDATION

Implement func (cfg *Config) Validate() error with a structured multi-error approach:
Return all validation errors at once (not just the first), so a misconfigured SDK 
fails with a complete error list rather than requiring iterative fix-and-retry.

Validation rules with exact error messages:
- CollectorEndpoint empty: "collector_endpoint is required; set OPENTRACE_COLLECTOR_ENDPOINT"
- CollectorEndpoint not valid URL: "collector_endpoint must be a valid URL with http/https scheme"
- ServiceName empty: "service_name is required; set OPENTRACE_SERVICE_NAME"
- BatchSize < 1 or > 5000: "batch_size must be between 1 and 5000, got N"
- BatchInterval < 100ms or > 60s: "batch_interval must be between 100ms and 60s, got Xms"
- HTTPTimeout < 1s or > 120s: "http_timeout must be between 1s and 120s, got Xs"

The New() constructor must call Validate() and return its error before spawning 
any goroutines — goroutines must never be started for an invalid configuration.

DELIVERABLE 5 — EXAMPLE USAGE FILE (examples/basic/main.go)

Write a complete, runnable example showing all three configuration approaches:
1. Fully programmatic (no env vars)
2. Fully environment-driven (no programmatic options beyond New())
3. Mixed (env vars for secrets, programmatic for app-specific settings)
Include comments explaining when to use each approach in real deployments.
```

---

### Day 28 — Example E-Commerce Service

**Intent:** Build a realistic mock service that exercises all SDK capabilities under simulated production load patterns.

```
ROLE: You are a Senior Backend Systems Engineer building a reference implementation 
that demonstrates SDK capabilities to enterprise adopters. This example must be 
realistic enough that a team evaluating OpenTrace can immediately see how it 
integrates into their existing Go services.

PROJECT CONTEXT:
Build a complete mock e-commerce microservice (examples/ecommerce-service/) that 
imports the OpenTrace Go SDK and uses it across realistic business logic flows. 
This service will serve as both a functional demonstration and a load generator 
for end-to-end testing.

FILE STRUCTURE:
examples/ecommerce-service/
├── main.go
├── handler/
│   ├── checkout.go
│   ├── products.go
│   └── orders.go
├── service/
│   ├── inventory.go
│   └── payment.go
├── middleware/
│   └── tracing.go
└── loadgen/
    └── loadgen.go

DELIVERABLE 1 — HTTP SERVER (main.go + handlers)

Implement a chi v5 HTTP server with these endpoints:

POST /checkout:
- Accepts: {"user_id": "...", "items": [{"product_id": "...", "quantity": N}]}
- Simulates: inventory check → payment processing → order creation
- Duration: simulate 50-200ms processing time (random)
- Log at INFO on success with fields: user_id, order_id, total_amount, item_count, duration_ms
- Log at WARN when inventory is low (simulated: 10% probability)
- Log at ERROR when payment fails (simulated: 5% probability)
- Log at DEBUG for each step (inventory_checked, payment_initiated, order_created)

GET /products?category=:category:
- Simulates: cache lookup → database query (with 20ms simulated latency)
- Log at INFO with: category, result_count, cache_hit (bool), duration_ms
- Log at WARN when result_count = 0 ("no products found for category")

GET /orders/:order_id:
- Simulates: order lookup with 15ms latency
- Log at ERROR if order not found (simulated: 15% of requests use invalid IDs)
- Log at INFO on success with: order_id, status, item_count

DELIVERABLE 2 — SDK INTEGRATION (middleware/tracing.go)

Implement a chi middleware that:
- Generates a request_id (UUID v4) and injects it into context
- Creates a child logger with pre-set fields: request_id, http.method, http.path, http.user_agent
- Passes the child logger via context (define a contextKey type)
- On request completion: logs a request summary at INFO with duration_ms and http.status_code
- If status >= 500: logs at ERROR with additional error details

Demonstrate logger.WithContext(ctx) integration so all handler log lines automatically 
include request_id without manual field passing.

DELIVERABLE 3 — LOAD GENERATOR (loadgen/loadgen.go)

Implement a concurrent load generator that runs as a goroutine within the same process:
- Configurable: rps (requests per second target) and duration
- Uses a time.Ticker to control request rate
- Distributes traffic across endpoints: 60% /checkout, 30% /products, 10% /orders
- Generates realistic randomized request bodies (10 product IDs, 100 user IDs, etc.)
- Tracks and logs: actual achieved RPS, error rate, p95 latency

DELIVERABLE 4 — STRUCTURED CONTEXT EXAMPLES

Demonstrate all SDK field types in context:
- String fields: user_id, order_id, product_id, payment_method
- Int fields: item_count, quantity, http.status_code
- Float64 fields: total_amount, tax_amount
- Bool fields: cache_hit, is_retry, requires_review
- Duration fields: db_query_time, payment_processing_time, total_duration
- Error fields: logger.Error("payment failed", opentrace.Error(err), ...)
- Nested context via logger.With(): create a payment-scoped logger with payment_id pre-set

Ensure this example is complete enough that someone can run it with:
  go run ./examples/ecommerce-service/ 
and see structured log events flowing through to the OpenTrace dashboard.
```

---

### Day 29 — SDK-to-Backend Pipeline Integration

**Intent:** Verify and document the complete end-to-end data flow from SDK event creation through network transport to PostgreSQL storage and React UI visibility.

```
ROLE: You are a Distributed Solutions Architect responsible for validating that 
all system components work correctly as an integrated system, not just in isolation. 
You design integration verification procedures that catch contract mismatches, 
serialization bugs, and configuration gaps before they reach production.

PROJECT CONTEXT:
Perform the first complete end-to-end integration of the OpenTrace system: 
the Go SDK generating events in the example e-commerce service, transmitting them 
to the collector-service, storing them in PostgreSQL, and rendering them in the 
React dashboard. Document and fix every integration gap found.

DELIVERABLE 1 — WIRE FORMAT ALIGNMENT VERIFICATION

Compare the SDK's outbound JSON payload structure against the collector's 
expected ingest schema (from Day 8). Document every field:

| SDK Field Path | Collector Expected Field | Type Match | Notes |
|---------------|--------------------------|------------|-------|
| event.Timestamp (time.Time) | events[].timestamp (string) | ✓ RFC3339 | |
| event.Level (Level type) | events[].level (string) | ✓ lowercase | |
| event.Fields[].Key | events[].attributes.{key} | ? | Flatten or nest? |
| resource.ServiceName | events[].resource.service.name | ? | OTel convention |
| ... | ... | ... | ... |

Identify any mismatches and provide the exact code changes required in either 
the SDK serializer or the collector handler to achieve exact alignment.

DELIVERABLE 2 — FIELD FLATTENING STRATEGY

Design and implement the field flattening logic in the collector:
The SDK sends fields as an array: [{"key": "user_id", "type": "string", "str_val": "u123"}]
The PostgreSQL schema stores attributes as JSONB: {"user_id": "u123"}

Implement func flattenFields(fields []SDKField) map[string]interface{} in the collector:
- Handle all SDK field types: string, int64, float64, bool, error, any
- Sanitize keys: max 255 characters, replace dots with underscores in non-OTel keys
- Handle key collisions (two fields with same key): append _2 suffix, log warning
- Test with the complete field set produced by the e-commerce service

DELIVERABLE 3 — END-TO-END TRACE VERIFICATION SCRIPT

Write a Go test program (cmd/e2e-verify/main.go) that:

Step 1 — GENERATE: Send 100 log events with unique, traceable content via the SDK:
  uniqueID := fmt.Sprintf("e2e-test-%d", time.Now().UnixNano())
  logger.Info("e2e verification event", opentrace.String("e2e_id", uniqueID))

Step 2 — WAIT: Sleep 5 seconds (allow batch flush + database write)

Step 3 — QUERY: Call GET /api/v1/logs?keyword={uniqueID} on the query-api

Step 4 — VERIFY: Assert that exactly 100 events are returned with matching e2e_id

Step 5 — REPORT: Print pass/fail with timing:
  ✓ Event generation: 100 events in 2.3ms
  ✓ Ingest latency: 100/100 events found after 4.7s
  ✓ Field integrity: all attributes preserved correctly
  ✗ FAILED: expected 100 events, found 97 (3 dropped by rate limiter?)

DELIVERABLE 4 — COMPRESSION CONFIGURATION ALIGNMENT

Verify the compression settings match end-to-end:
- SDK sends gzip-compressed payloads when batch > 1KB (Day 26 implementation)
- Collector must have Content-Encoding: gzip decoding middleware
- Implement and document the middleware in collector-service/internal/middleware/decompress.go
- Verify: disable compression in SDK and verify collector still works (regression test)
- Verify: enable compression and confirm Content-Length is smaller

DELIVERABLE 5 — CONFIGURATION CONSISTENCY CHECKLIST

Produce a configuration alignment checklist validating all shared settings:
[ ] SDK CollectorEndpoint matches COLLECTOR_PORT in docker-compose.yml
[ ] SDK BatchSize (500) <= Collector max batch size (1000) — never exceed server limit
[ ] SDK HTTP timeout (10s) < Collector WriteTimeout (10s) — client must time out first
[ ] Content-Type: application/json set by SDK, expected by collector
[ ] X-SDK-Version header passes through collector to database log_attributes
[ ] Error responses from collector are correctly parsed by SDK retry logic
```

---

### Day 30 — Real-Time Validation & Performance Verification

**Intent:** Execute a comprehensive system health validation playbook and establish the baseline performance metrics for the complete integrated pipeline.

```
ROLE: You are a Senior Site Reliability Engineer who owns the production readiness 
gate for the OpenTrace platform. You do not trust the system works correctly 
until you have traced data through every component and measured latency at every 
hop with instrumented verification scripts.

PROJECT CONTEXT:
Execute a complete end-to-end performance and correctness validation of the 
OpenTrace pipeline: SDK → collector-service → PostgreSQL → query-api → React UI. 
Produce a reusable validation playbook that can be run as a smoke test after 
every deployment.

DELIVERABLE 1 — LATENCY MEASUREMENT PLAYBOOK

Define measurement points and instrumentation for each pipeline hop:

HOP 1: SDK buffer enqueue latency
  Measure: time from logger.Info() call to event in internal buffer
  Target: < 1 microsecond
  How: Go benchmark in sdk/go/logger_bench_test.go with b.ReportAllocs()

HOP 2: SDK batch flush to HTTP transmission start
  Measure: time from batch trigger to first byte of HTTP request
  Target: < 5ms
  How: Add timing instrumentation to the exporter when Debug=true

HOP 3: HTTP transmission to collector 202 response
  Measure: end-to-end HTTP round-trip time
  Target: < 50ms (local), < 100ms (cloud)
  How: Capture from SDK exporter timing log; correlate with collector access log

HOP 4: Collector to PostgreSQL commit
  Measure: time from handler receive to database commit completion
  Target: < 100ms for 100-event batch
  How: Add timing to the repository layer with slog

HOP 5: Database commit to query-api visibility
  Measure: time from INSERT commit to SELECT returning the row
  Target: < 10ms (PostgreSQL read-your-writes guarantee)
  How: e2e-verify script timestamp comparison

TOTAL PIPELINE LATENCY TARGET: < 2 seconds from SDK call to React UI visibility

DELIVERABLE 2 — OPERATIONAL DEBUGGING QUERIES

Provide ready-to-run SQL queries for common debugging scenarios:

a) Verify recent ingest is working:
   SELECT COUNT(*), MAX(received_at), NOW() - MAX(received_at) AS lag
   FROM logs
   WHERE received_at > NOW() - INTERVAL '5 minutes';
   Expected: count > 0, lag < 30 seconds

b) Detect data gaps (missing time windows):
   SELECT generate_series(
     date_trunc('minute', NOW() - INTERVAL '1 hour'),
     date_trunc('minute', NOW()),
     INTERVAL '1 minute'
   ) AS minute,
   COUNT(l.id) AS event_count
   FROM (SELECT generate_series(...) AS minute) t
   LEFT JOIN logs l ON date_trunc('minute', l.timestamp) = t.minute
   GROUP BY t.minute ORDER BY t.minute;

c) Detect schema mismatches (events missing required fields):
   SELECT COUNT(*) FROM logs WHERE service_name IS NULL OR body IS NULL;
   Expected: 0

d) Measure write throughput over past 5 minutes:
   SELECT 
     date_trunc('second', received_at) AS second,
     COUNT(*) AS events_per_second
   FROM logs
   WHERE received_at > NOW() - INTERVAL '5 minutes'
   GROUP BY 1 ORDER BY 1;

DELIVERABLE 3 — TELEMETRY MISMATCH RUN-BOOK

Document the diagnostic decision tree for the most common failure modes:

SYMPTOM: SDK shows events sent, but nothing in PostgreSQL
Step 1: Check collector-service logs for 4xx errors (payload validation failure?)
Step 2: Check collector-service logs for database errors (connection pool exhausted?)
Step 3: Verify compression alignment: collector can decode gzip? (curl -H "Content-Encoding: gzip")
Step 4: Verify batch is not being dropped by rate limiter (check X-RateLimit-Remaining header)
Step 5: Check PostgreSQL connection pool status (pgxpool stats endpoint)

SYMPTOM: Events in PostgreSQL but not visible in React UI
Step 1: Verify query-api is connected to the same PostgreSQL instance (check logs)
Step 2: Run the debug query (b) above to check for time range issues
Step 3: Verify UI time range filter includes the event timestamps
Step 4: Check for cursor pagination issues (query may be on wrong page)
Step 5: Verify severity filter is not excluding the events

SYMPTOM: High dropped event count in SDK
Step 1: Check SDK buffer utilization: is circuit breaker open?
Step 2: Check collector p99 response time: is it > SDK HTTP timeout?
Step 3: Check SDK BatchInterval vs event generation rate: batch too small for rate?
Step 4: Check collector resource limits: is it CPU-throttled in Docker?

DELIVERABLE 4 — PERFORMANCE BASELINE REPORT TEMPLATE

Produce a markdown template for the performance baseline report that must be 
completed and committed to the repository after this validation:

# OpenTrace Performance Baseline — {DATE}

## Environment
- Go SDK version: 
- Collector version:
- PostgreSQL version:
- Test machine specs:

## Throughput Measurements
| Metric | Measured Value | Target | Status |
|--------|---------------|--------|--------|
| SDK log calls/sec (single goroutine) | | 1,000,000 | |
| SDK log calls/sec (100 goroutines) | | 500,000 | |
| Collector ingest rate (events/sec) | | 10,000 | |
| PostgreSQL insert rate (rows/sec) | | 50,000 | |

## Latency Measurements (p50/p95/p99)
[Table for each pipeline hop]

## Resource Utilization at 1,000 events/sec sustained
[CPU%, memory MB for each service]
```

---

### Day 31 — Structured Logging Schema Upgrade

**Intent:** Upgrade the SDK and storage layer to support arbitrarily nested structured metadata without sacrificing query performance or schema stability.

```
ROLE: You are a Staff Systems Engineer who designs data schemas that must remain 
queryable for years after the data is written, even as application attribute schemas 
evolve. You treat schema evolution as a first-class engineering concern, not an afterthought.

PROJECT CONTEXT:
Upgrade the OpenTrace SDK and PostgreSQL storage layer to fully support deeply nested 
structured metadata attributes. The current system stores attributes as a flat JSONB map. 
The upgrade must support nested objects like {"user": {"id": "123", "role": "admin", 
"permissions": {"billing": true}}} without breaking existing flat-attribute queries.

DELIVERABLE 1 — SDK INTERFACE EXTENSION

Extend the SDK Field constructors to support nested attributes:

New field types to support:
opentrace.Object(key string, fields ...Field) Field  // nested object
opentrace.StringSlice(key string, values []string) Field  // array of strings
opentrace.Map(key string, m map[string]string) Field  // string map

Usage example:
logger.Info("user action",
    opentrace.String("action", "checkout"),
    opentrace.Object("user",
        opentrace.String("id", userID),
        opentrace.String("email", email),
        opentrace.Object("permissions",
            opentrace.Bool("billing", true),
            opentrace.Bool("admin", false),
        ),
    ),
    opentrace.Object("http",
        opentrace.String("method", "POST"),
        opentrace.Int("status", 200),
    ),
)

Implement the Field serialization for Object type: recursively serialize to 
nested JSON using the zero-allocation approach from Day 23 (write directly 
to a bytes.Buffer rather than marshaling through interface{}).

DELIVERABLE 2 — COLLECTOR SANITIZATION LAYER

Implement func sanitizeAttributes(raw map[string]interface{}) (map[string]interface{}, []string):
Returns: sanitized map and a slice of warning messages for any sanitization actions taken.

Rules:
- Maximum nesting depth: 5 levels (flatten deeper nesting to dot-notation keys)
  {"a": {"b": {"c": {"d": {"e": {"f": "too deep"}}}}}} 
  → {"a.b.c.d.e.f": "too deep"} with warning
- Maximum total key count: 200 (drop keys beyond this with warning)
- Key name max length: 255 characters (truncate with warning)
- Key must match regex [a-zA-Z][a-zA-Z0-9_.]*: replace invalid chars with _ with warning
- Value max string length: 32,768 characters (truncate with warning)
- Prohibited key names: block OTel reserved prefixes that the SDK sets automatically
  (e.g., host., service., process.) when they come from user attributes

DELIVERABLE 3 — POSTGRESQL JSONB QUERY OPTIMIZATION

Show how to query deeply nested JSONB attributes efficiently:

a) Query by nested value (find events where user.role = 'admin'):
   Naive (triggers GIN scan but may be slow for deep paths):
   SELECT * FROM logs WHERE log_attributes @> '{"user": {"role": "admin"}}'::jsonb;
   
   Show how to verify this uses the GIN index: EXPLAIN ANALYZE the query.
   
   Show the jsonb_path_ops vs jsonb_ops operator class choice impact for this query.

b) Generated column for high-frequency nested field queries:
   Show how to add a generated column for a frequently-queried nested field:
   ALTER TABLE logs ADD COLUMN user_id TEXT 
     GENERATED ALWAYS AS (log_attributes->>'user_id') STORED;
   CREATE INDEX ON logs (user_id) WHERE user_id IS NOT NULL;
   
   Explain the trade-off: faster queries, but the column must be defined at schema time 
   (can't do this for arbitrary user-defined attributes).

c) Partial indexes for common attribute combinations:
   CREATE INDEX ON logs (service_name, (log_attributes->>'user_id'))
   WHERE log_attributes ? 'user_id';
   
   Explain when this is better than a GIN index.

DELIVERABLE 4 — MIGRATION STRATEGY

Write the migration script and rollback plan for adding nested attribute support 
to an existing production deployment:
- Schema changes: no DDL changes required (JSONB already supports nesting)
- Application changes: new SDK version supports nested fields; old version still works
- Query changes: add the generated column migration as a non-blocking ALTER TABLE
- Rollback: generated column can be dropped without data loss
- Feature flag: how to enable nested attribute support in the SDK without forcing 
  all adopters to upgrade at once
```

---

### Day 32 — SDK Performance Optimization

**Intent:** Apply systematic profiling and mechanical sympathy techniques to reduce the SDK's CPU and memory overhead to the absolute minimum achievable in Go.

```
ROLE: You are an Elite Go Performance Optimization Engineer with expertise in 
profiling, assembly output analysis, and mechanical sympathy. You treat every 
heap allocation on a hot path as a bug, not a trade-off.

PROJECT CONTEXT:
The OpenTrace Go SDK is adding measurable overhead to host applications under 
extreme load (100,000+ log calls/second across 1,000 goroutines). Apply systematic 
optimization techniques to reduce CPU overhead and heap allocations to the absolute 
minimum without compromising correctness.

CURRENT BASELINE (provide after running):
go test -bench=BenchmarkLoggerInfo -benchmem -count=5 ./sdk/go/...

DELIVERABLE 1 — SYNC.POOL OPTIMIZATION AUDIT

Audit the current sync.Pool usage and identify inefficiencies:

a) Pool thrashing analysis:
   sync.Pool objects are cleared on GC. If GC runs frequently (high allocation rate), 
   pool objects are not reused. Show how to measure pool hit rate using runtime/metrics:
   /gc/heap/allocs:bytes — measure before and after pool implementation

b) Object size optimization:
   The LogEvent struct allocated from the pool must fit in a single cache line (64 bytes) 
   where possible. Show the current struct layout and propose field reordering:
   go tool compile -S sdk/go/event.go | grep "size="
   
   Propose a union-like value storage approach where all scalar Field values 
   share the same 8-byte slot using unsafe.Pointer or a uint64 with type assertions.

c) Fields slice pre-allocation:
   The Fields []Field slice in LogEvent must be pre-allocated to the right capacity 
   to avoid growth copies. Analyze the typical fields-per-event distribution from 
   the e-commerce service benchmark and set the initial capacity accordingly.

DELIVERABLE 2 — ZERO-ALLOCATION JSON SERIALIZATION

Replace encoding/json in the batch serializer with a zero-copy approach:

Option A — jsoniter library:
   Benchmark encoding/json vs github.com/json-iterator/go vs encoding/json with 
   a go:generate directive to pre-generate decoders.
   Show the benchmark comparison for a 100-event batch serialization.

Option B — Manual builder with bytes.Buffer:
   Implement a hand-written JSON serializer for LogEvent that writes directly 
   to a pre-allocated bytes.Buffer without reflection.
   Show the implementation for a complete LogEvent to JSON string.
   
   This is more code but achieves lower allocations by eliminating the reflect 
   type metadata lookups. Benchmark both options and recommend one based on 
   the allocation profile.

DELIVERABLE 3 — STRING INTERNING FOR REPEATED KEYS

In high-volume logging, the same field keys (e.g., "user_id", "request_id", 
"service_name") are allocated as new strings on every log call. Implement 
string interning to reuse string allocations:

type internedString struct {
    s string
}
var internedStrings sync.Map  // map[string]*internedString

func intern(s string) string — return a cached pointer to the same string value.

Show the allocation reduction via benchmark:
BenchmarkFieldKey_NoIntern: X allocs/op
BenchmarkFieldKey_WithIntern: Y allocs/op (expected: 0 for cached keys)

DELIVERABLE 4 — PPROF PROFILING RUN-BOOK

Write a complete profiling procedure that any engineer can follow:

Step 1 — CPU Profile:
  go test -bench=BenchmarkLoggerInfo -cpuprofile=cpu.prof ./sdk/go/...
  go tool pprof -http=:8080 cpu.prof
  In the web UI: navigate to Graph view, identify functions with highest "self" time.
  Expected hotspots: time.Now(), json encoding, channel send.

Step 2 — Memory Allocation Profile:
  go test -bench=BenchmarkLoggerInfo -memprofile=mem.prof ./sdk/go/...
  go tool pprof -alloc_objects -http=:8080 mem.prof
  Identify the top 5 allocation sites. Each one above 0 allocations/op on the 
  hot path is a candidate for optimization.

Step 3 — Heap Escape Analysis:
  go build -gcflags="-m=2" ./sdk/go/... 2>&1 | grep "escapes to heap"
  Each "escapes to heap" on a hot path function is a hidden allocation.
  Show how to fix interface boxing escapes by using concrete types.

Step 4 — Trace visualization:
  go test -bench=BenchmarkLoggerInfo -trace=trace.out ./sdk/go/...
  go tool trace trace.out
  Navigate to: Goroutine analysis → find goroutine blocking patterns.
  Identify if the background exporter goroutine is blocking the main goroutine.
```

---

### Day 33 — Graceful Shutdown Implementation

**Intent:** Implement a bulletproof shutdown sequence that guarantees zero data loss for in-flight events when the host application receives a termination signal.

```
ROLE: You are a Senior Go Concurrency and Lifecycle Architect who has debugged 
data loss incidents caused by incomplete shutdown sequences in production logging 
systems. You treat graceful shutdown as a critical correctness feature, not an 
operational nicety.

PROJECT CONTEXT:
Implement the complete graceful shutdown orchestration for the OpenTrace Go SDK. 
The shutdown sequence must guarantee that no log events that were successfully 
passed to logger.Info() are lost when the host application receives SIGTERM or 
the caller invokes logger.Shutdown(ctx).

DELIVERABLE 1 — SHUTDOWN SEQUENCE DESIGN

Document the exact shutdown sequence as a numbered ordered list:

1. Signal received (SIGTERM/SIGINT) or Shutdown() called externally
2. Set accepting=false (atomic bool): new calls to logger.Info() return immediately with dropped count++
3. Close the input channel to the batcher (signals batcher to flush final batch)
4. Wait for batcher to flush all in-flight events to the flush channel (batcher WaitGroup)
5. Close the flush channel to the exporter (signals exporter to drain and stop)
6. Exporter drain loop: process all remaining batches in the flush channel
7. For each remaining batch: attempt Export() with the shutdown context deadline
8. If Export() fails during shutdown: log the loss count and continue (don't hang)
9. Wait for exporter goroutine to complete (exporter WaitGroup)
10. Close HTTP client idle connections
11. Log: "OpenTrace SDK shutdown complete. N events transmitted, M events dropped."
12. Return nil (or error if deadline exceeded)

DELIVERABLE 2 — SIGNAL HANDLING (optional SDK-managed)

Implement func (l *Logger) RegisterSignalHandler():
- Creates a signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
- On signal: calls l.Shutdown(ctx) with 30-second timeout
- Designed for applications that don't have their own signal handling
- Document clearly that this is optional — applications with their own shutdown 
  logic should call l.Shutdown(ctx) directly from their own signal handler

DELIVERABLE 3 — SHUTDOWN IMPLEMENTATION (sdk/go/logger.go)

func (l *Logger) Shutdown(ctx context.Context) error:
- Use sync.Once to ensure shutdown is idempotent (safe to call multiple times)
- Track shutdown state with atomic bool: l.isShutdown.Store(true)
- Implement the sequence from Deliverable 1 precisely
- Respect the ctx deadline at every step: if ctx expires, log the current state 
  and return context.DeadlineExceeded (do not block indefinitely)
- Collect any errors from each step and return them as a combined error

Integration in all logger.Xxx() methods:
  if l.isShutdown.Load() {
      l.dropped.Add(1)
      return  // fast path for post-shutdown calls
  }

DELIVERABLE 4 — SHUTDOWN TIMEOUT ANALYSIS

Derive the recommended shutdown timeout value:

Given:
- Max batch size: 500 events
- HTTP timeout: 10 seconds
- Max retries: 5 with exponential backoff up to 30s max delay
- Worst case time to drain 1 batch: 10s (timeout) × 5 retries = 50s

This means a full retry cycle exceeds any reasonable shutdown timeout. 
Define the shutdown behavior policy:
- Shutdown context timeout: 15 seconds (configurable via WithShutdownTimeout())
- During shutdown: reduce max retries to 2 (prioritize speed over persistence)
- After timeout: report how many events were in-flight when abandoned

Document this trade-off explicitly: a 15-second shutdown with max 2 retries means 
losing events if the collector is down for > ~4 seconds at shutdown time. 
State this as a known limitation in the SDK documentation.

DELIVERABLE 5 — SHUTDOWN INTEGRATION TEST

Write an integration test (sdk/go/shutdown_test.go) using testcontainers:

Test 1 — Clean shutdown:
  Start a mock HTTP server (collector), send 1,000 events, call Shutdown(ctx).
  Verify: mock server received all 1,000 events within shutdown deadline.

Test 2 — Shutdown with slow collector:
  Mock server delays each response by 100ms.
  Send 5,000 events, call Shutdown(ctx) with 5-second deadline.
  Verify: shutdown completes within 5 seconds (deadline respected).
  Verify: dropped count + received count = 5,000 (no events unaccounted for).

Test 3 — Shutdown with dead collector:
  Mock server stops accepting connections.
  Send 100 events, call Shutdown(ctx) with 3-second deadline.
  Verify: shutdown returns within 3 seconds + small epsilon.
  Verify: returns context.DeadlineExceeded or ErrCollectorUnreachable.
  Verify: no goroutine leaks (goleak.VerifyNone(t) after shutdown).
```

---

### Day 34 — SDK Load Testing & Breaking Point Analysis

**Intent:** Execute a comprehensive stress test suite that determines the SDK's absolute performance ceiling and identifies every bottleneck with precision.

```
ROLE: You are a Performance and Scalability Engineer who designs stress tests with 
the goal of finding failures in controlled conditions rather than production. 
You treat benchmark results as engineering specifications — they set expectations 
that the implementation must meet or the implementation must change.

PROJECT CONTEXT:
Design and execute a rigorous stress test suite for the OpenTrace Go SDK. 
The goal is to determine: maximum sustainable throughput, memory behavior under 
sustained load, and exact failure mode when the SDK reaches its capacity limits.

DELIVERABLE 1 — THROUGHPUT BENCHMARK SUITE

Write Go benchmarks (sdk/go/bench_test.go) designed to isolate each component:

BenchmarkSDK_SingleGoroutine:
  b.RunParallel with GOMAXPROCS=1
  Target: >= 1,000,000 log calls/second
  Measure: allocs/op (target: 0), bytes/op (target: 0), ns/op

BenchmarkSDK_Parallel_10:
  10 parallel goroutines
  Target: >= 5,000,000 log calls/second aggregate

BenchmarkSDK_Parallel_100:
  100 parallel goroutines
  Target: should not degrade proportionally (shows channel contention if it does)

BenchmarkSDK_WithFields_5:
  5 mixed-type fields per call
  Compare: allocs/op vs single-call baseline (must remain 0)

BenchmarkSDK_WithObject_Nested:
  Nested Object field with 3 levels
  Expected: 1-2 allocs/op (unavoidable for nested serialization)

DELIVERABLE 2 — SUSTAINED LOAD TEST (loadtest/main.go)

Write a standalone load test program (not a Go benchmark) that:
- Runs for 5 minutes at configurable target RPS
- Uses 100 goroutines generating logs concurrently
- Collects metrics every 10 seconds:
  * Actual throughput (events/sec)
  * SDK internal buffer utilization (%)
  * Dropped event count (cumulative)
  * Host process memory (runtime.MemStats)
  * GC pause time (runtime.ReadMemStats)
- Sends to a mock HTTP server that intentionally delays at configurable latency
- Reports final statistics: peak throughput, mean throughput, total dropped, 
  memory growth over test duration

DELIVERABLE 3 — BREAKING POINT IDENTIFICATION

Run the sustained load test with escalating conditions and document the exact breaking point:

Scenario A — Buffer Saturation:
  Throttle the mock collector to 100ms response time.
  Ramp RPS from 1,000 to 100,000 in 1,000 RPS increments.
  Record the RPS at which dropped events first appear (= buffer saturation point).

Scenario B — Memory Pressure:
  Run at 50,000 RPS sustained for 60 minutes.
  Record memory growth curve: should be flat after warm-up (no memory leak).
  Alert threshold: > 10MB growth after 10-minute warm-up.

Scenario C — GC Impact:
  Run at 100,000 RPS with GOGC=100 (default) vs GOGC=400.
  Measure: allocation rate, GC frequency, GC pause time p99.
  Show that 0-alloc hot path dramatically reduces GC pressure.

DELIVERABLE 4 — BOTTLENECK REMEDIATION GUIDE

Document the symptoms and remediation for each discovered bottleneck:

BOTTLENECK: Channel send blocking
  Symptom: BenchmarkSDK_Parallel_100 throughput << BenchmarkSDK_Parallel_10 × 10
  Diagnosis: Buffered channel is full; goroutines blocking on send
  Remediation: Increase buffer size, or switch to lock-free ring buffer

BOTTLENECK: sync.Pool contention
  Symptom: High runtime.lock contention in CPU profile
  Diagnosis: sync.Pool under high concurrency has lock contention on the per-P pools
  Remediation: Pre-shard the pool, or batch-acquire objects

BOTTLENECK: GC pauses disrupting throughput
  Symptom: Throughput spikes/valleys correlating with runtime.GCStats.PauseTotal growth
  Diagnosis: Allocations on hot path triggering frequent short GC cycles
  Remediation: Eliminate remaining allocs, tune GOGC=off + manual GC trigger

BOTTLENECK: HTTP client connection pool exhaustion
  Symptom: Export latency increases under sustained load, seeing "no free connections" errors
  Diagnosis: max_idle_conns exceeded; creating new TCP connections per request
  Remediation: Increase http.Transport.MaxIdleConns, ensure response bodies always drained
```

---

### Day 35 — Phase 2 Final Architecture Review

**Intent:** Execute the final, comprehensive review of the complete Phase 2 SDK and ingestion pipeline before transitioning to the Redpanda/Kafka scaling phase.

```
ROLE: You are a Distinguished Principal Engineer with the authority to approve or 
block a system's transition from initial implementation to production-scale 
infrastructure. You have shipped observability platforms at companies where 
a single data loss incident requires an RCA document read by the CEO. 
Your review leaves no assumption unchallenged.

REVIEW SCOPE:
The complete Phase 2 implementation of the OpenTrace SDK and collector pipeline:
- Go SDK (opentrace-go): public API, field types, pool, batcher, exporter, retry, 
  circuit breaker, config, graceful shutdown
- OpenTrace collector-service: ingest handler, validation, PostgreSQL persistence
- End-to-end integration: wire format alignment, compression, e2e verification script
- Load test results: throughput benchmarks, breaking point analysis

[PASTE ALL SDK AND INTEGRATION CODE HERE]
[PASTE BENCHMARK RESULTS FROM DAY 34 HERE]

REVIEW SECTION 1 — SDK API ERGONOMICS & CLEANLINESS

Evaluate the public API surface against these criteria:
a) Discoverability: Can a Go engineer understand how to use the SDK 
   from just the exported types and godoc comments, without reading the README?
   If not: identify every exported symbol that needs better documentation.

b) Misuse prevention: Identify any API design that could lead to silent incorrect behavior:
   - Can a caller create a Logger that never flushes without realizing it?
   - Is there any way to call Shutdown() and then continue logging without an error?
   - Can the caller exhaust the host process memory through SDK misuse?

c) Testing ergonomics: How easy is it to use the SDK in unit tests?
   - Is there a way to verify what log events were emitted in a test?
   - Does the NopLogger correctly satisfy all interfaces needed by test code?

d) Configuration discoverability: Can operators understand all config options 
   without reading source code? Evaluate the functional options documentation.

REVIEW SECTION 2 — CONCURRENCY HARDENING

Perform a detailed concurrency audit:

a) Identify every shared mutable state in the SDK and verify it is correctly protected:
   Map every data structure to its synchronization primitive (atomic, mutex, channel, sync.Pool)
   and the goroutines that can access it concurrently.

b) Deadlock analysis:
   Document every acquire order (mutex A → mutex B) in the codebase.
   Verify no code path acquires them in the reverse order (mutex B → mutex A).
   If any lock ordering risk exists: prescribe the exact fix.

c) Goroutine lifecycle audit:
   List every goroutine spawned by the SDK.
   For each: what starts it, what stops it, what prevents it from leaking on error.
   Run go test -run=. -race ./sdk/go/... and confirm zero race conditions.

d) Shutdown race analysis:
   Can a goroutine still be writing to the batch buffer after Shutdown() signals the 
   batcher to stop? Trace the exact happens-before relationship between:
   isShutdown.Store(true) → buffer write → batcher close → exporter drain

REVIEW SECTION 3 — PRODUCTION READINESS SCORE

Score the SDK on each dimension (0-10, with 10 = production-ready at enterprise scale):

| Dimension | Score | Critical Gaps |
|-----------|-------|---------------|
| Performance (throughput, allocations, latency) | /10 | |
| Correctness (data integrity, ordering guarantees) | /10 | |
| Reliability (graceful shutdown, retry, circuit breaker) | /10 | |
| Observability (SDK self-telemetry, dropped counter, debug mode) | /10 | |
| Ergonomics (API clarity, testing support, documentation) | /10 | |
| Security (no sensitive data leaks, safe config handling) | /10 | |
| Operational maturity (versioning, changelog, migration guide) | /10 | |

REVIEW SECTION 4 — PHASE 3 PREREQUISITE CHECKLIST

Before the Redpanda/Kafka scaling phase begins, the following must be true. 
Evaluate each as PASS/FAIL with evidence:

[ ] SDK allocates 0 bytes/op on logger.Info() with string fields (benchmark evidence required)
[ ] Zero data race errors (go test -race evidence required)
[ ] Graceful shutdown verified to drain all in-flight events (integration test evidence)
[ ] End-to-end latency p99 < 2 seconds under 1,000 events/sec (e2e-verify evidence)
[ ] API contract between SDK and collector is formally documented and version-pinned
[ ] All public SDK functions have godoc comments
[ ] All exported errors are sentinel errors (errors.Is() compatible)
[ ] Config validation rejects all known dangerous configurations at startup

FINAL OUTPUT:
A Go/No-Go decision for Phase 3, with:
- List of BLOCKING issues (must be resolved before any Phase 3 work begins)
- List of TECHNICAL DEBT items (track in GitHub issues, resolve within 2 weeks)
- A one-paragraph architectural summary suitable for sharing with the engineering org
```

---

*End of OpenTrace CLI Prompt Library — Phase 0 through Phase 2*

---

> **Usage Note:** Each prompt above is self-contained and can be pasted directly into any CLI AI assistant. Replace `[PASTE ... HERE]` placeholders with your actual implementation artifacts. The prompts follow a deliberate progression — earlier phase outputs become inputs to later phase reviews.
