# ADR-001: Monorepo Structure and Module Boundaries

**Status:** Accepted  
**Date:** Phase 0

## Context

OpenTrace comprises three distinct deployable units (collector-service, query-api, opentrace-ui) and a separately-versioned client SDK. The structural decision is whether to use a monorepo with Go Workspaces or multiple repositories.

## Decision

Single monorepo with Go Workspaces (`go.work`) covering two Go modules:
1. `github.com/opentrace/opentrace` — all backend services
2. `github.com/opentrace/opentrace-go` — the Go SDK (independently versioned)

## Rationale

**`internal/` vs `pkg/` boundary:**  
Go's `internal/` convention is a compile-time enforcement mechanism, not just a naming convention. Code in `internal/collector-service/` cannot be imported by `internal/query-api/` or by the SDK. This forces each service to own its dependencies explicitly and prevents the "shared util" anti-pattern that creates hidden coupling between services.  
`pkg/` contains types that are intentionally shared across service boundaries — primarily `pkg/schema` (the canonical wire format types). Any change to `pkg/` is a cross-service breaking change and must be treated as such.

**Protobuf/API contracts in `/api/`:**  
Schema definitions live in `/api/v1/` rather than inside each service to enforce a single-owner rule: the API is the contract, not the implementation. When a service needs to change a field, it modifies the contract first and then updates all consumers, preventing the drift that happens when each service embeds its own "internal" schema definition.

**Go Workspaces:**  
`go.work` enables `replace` directives without modifying individual `go.mod` files, so `sdk/go` can reference internal packages during development while still being tagged independently for external consumers. SDK v1.2.3 can be pinned by downstream users regardless of the backend version.

**Monorepo vs. separate infra repository:**  
Co-locating infrastructure code (Docker Compose, Kubernetes manifests, Helm charts) with application code in `/infra/` means the infrastructure definition is versioned alongside the code it deploys. A Git tag covers both the service binary and the corresponding Helm chart values, making rollbacks deterministic. The trade-off is that teams with dedicated platform engineering may prefer a separate infra repo; that split can happen without restructuring the application code.

## Consequences

- New services must follow the `cmd/<name>/main.go` + `internal/<name>/` layout.
- `pkg/schema` changes require reviewing all import sites in both services.
- SDK releases use semantic versioning independently of the backend release cycle.
