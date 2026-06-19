# Phase 1 Pre-Production Readiness Review

Audit performed against all code committed in Days 8–20.  
Status: **PASS** / **WARN** / **FAIL** for each item.  
All FAILs must be resolved before Phase 1 is merged to `main`.

---

## 1. Concurrency and Race Conditions

| # | File / Location | Finding | Severity | Status |
|---|-----------------|---------|----------|--------|
| 1 | `repository/logs_postgres.go:46–83` | `sync.WaitGroup.Add` called before `Begin` — if `pool.Begin` panics the `wg.Done` deferred call still fires, leaving the counter balanced. Correct. | Low | PASS |
| 2 | `hooks/useAutoRefresh.ts` — `enabled.current` is read inside `useEffect` closure but the effect only re-runs when `[enabled, visible, intervalMs]` changes. Since `enabled` is a `ref`, changing it via `pause()`/`resume()` does not trigger a re-render and the interval is **not** restarted. | Medium | WARN |
| 3 | `store/filterStore.ts` — Zustand store is a single shared singleton; state mutations on the auto-refresh interval happen on the React event loop (single-threaded). No race. | — | PASS |
| 4 | `sdk/go/buffer.go` — non-blocking channel send with `default` branch is correctly lock-free. Dropped events are an intentional back-pressure mechanism. | — | PASS |

**Remediation for #2:** Change `usePolling` to expose `paused` as a React state boolean and thread it into `useAutoRefresh`'s `enabled` parameter so the effect dependency array captures the change correctly.

```tsx
// After fix:
export function usePolling(intervalMs: number, onRefresh: () => void) {
  const [paused, setPaused] = useState(false);
  useAutoRefresh({ intervalMs, onRefresh, enabled: !paused });
  return { pause: () => setPaused(true), resume: () => setPaused(false) };
}
```

---

## 2. SQL Correctness

| # | File / Location | Finding | Severity | Status |
|---|-----------------|---------|----------|--------|
| 5 | `repository/logs_query_test.go:TestBuildLogsQuery_ParameterNumbering` | Test checks `$1..$n` by converting loop index to a single rune character — fails for `n ≥ 10` (`$10` → checks for `$1` + `$0`). Does not affect production code. | Low | WARN |
| 6 | `repository/logs_postgres.go:buildLogsQuery` | All user values bound via `ph()` — no string interpolation. SQL injection structurally impossible. | — | PASS |
| 7 | `infra/postgres/init/001_logs_schema.sql` | `body_tsv` is a `GENERATED ALWAYS AS STORED` column — cannot be INSERT-targeted. `logColumns` in the Go repo correctly excludes it. | — | PASS |
| 8 | Cursor comparison: `(timestamp, id) > ($n, $m)` — PostgreSQL row comparison is semantically correct for keyset pagination on `(timestamp ASC, id ASC)`. | — | PASS |
| 9 | `logs_query.go:handler` — `level` filter uses `::log_severity` cast. If an unmapped severity string is passed, PostgreSQL raises `22P02 invalid input value`. The handler validates against `LogLevel.Validate()` before the query executes, so only known values reach the DB. | — | PASS |

**Remediation for #5:**
```go
// Replace single-rune conversion with fmt.Sprintf:
placeholder := fmt.Sprintf("$%d", i)
```

---

## 3. React Memory Leaks

| # | File / Location | Finding | Severity | Status |
|---|-----------------|---------|----------|--------|
| 10 | `hooks/usePageVisibility.ts` — `removeEventListener` cleanup is registered in the effect return. | — | PASS |
| 11 | `hooks/useAutoRefresh.ts` — `clearInterval(id)` in effect return. | — | PASS |
| 12 | `components/logs/LogTable.tsx` — `rowVirtualizer` references `parentRef.current`. If the component unmounts while the virtualizer is mid-frame, accessing `parentRef.current` returns null. `useVirtualizer` handles this safely by checking for null internally. | — | PASS |
| 13 | `ApiErrorBoundary.tsx` — class component; `reset` method is an arrow function assigned in the class body. Each render creates a new function identity but the method is only referenced in JSX, not in a dependency array. Not a leak, though a minor allocation. | Low | PASS |

---

## 4. Security Audit

| # | Area | Finding | Severity | Status |
|---|------|---------|----------|--------|
| 14 | Ingest handler | `MaxBytesReader` limits payload to `maxPayloadBytes`. Oversized requests return 413 before any JSON decode. | — | PASS |
| 15 | Query handler | `keyword` parameter length-capped at 256 chars (`parseLogsQueryParams`). Long FTS queries cannot cause excessive CPU. | — | PASS |
| 16 | Cursor parameter | Cursor is base64-decoded and JSON-unmarshalled; if either step fails, 400 is returned. The cursor value is never used as a string in SQL — it is decomposed into `(timestamp, id)` which are bound as typed parameters. | — | PASS |
| 17 | Nginx CSP | `Content-Security-Policy: default-src 'self'` in `nginx.conf`. `style-src 'self' 'unsafe-inline'` — CSS modules inject styles via `<style>` tags, requiring `unsafe-inline`. Consider adding a nonce-based CSP for production. | Medium | WARN |
| 18 | Secrets in env | `docker-compose.yml` passes `POSTGRES_PASSWORD` via `${POSTGRES_PASSWORD}` from `.env`. `.env.example` has `changeme_in_production`. `.dockerignore` and `.gitignore` exclude `.env`. | — | PASS |
| 19 | SQL injection | All parameterised; no `fmt.Sprintf` in SQL paths. | — | PASS |
| 20 | Docker image | `Dockerfile.go` uses `distroless:nonroot` — no shell, minimal attack surface. `Dockerfile.ui` runs as `nginx` user. | — | PASS |
| 21 | CORS | Neither `collector-service` nor `query-api` sets `Access-Control-Allow-Origin`. All API calls go through the Nginx proxy on the same origin. No CORS needed. Correct. | — | PASS |

**Remediation for #17 (Medium — recommended before production traffic):**  
Add a Vite plugin (e.g. `vite-plugin-csp-nonce`) that injects a per-request nonce into both the CSP header and any inline `<style>` elements generated by CSS Modules, then remove `'unsafe-inline'` from the policy.

---

## 5. Remediation Matrix

| # | Priority | Owner | Action | ETA |
|---|----------|-------|--------|-----|
| 2 | P1 | Frontend | Fix `usePolling` to use React state instead of ref | Phase 2 Day 1 |
| 5 | P2 | Backend | Fix `TestBuildLogsQuery_ParameterNumbering` for n ≥ 10 | Phase 2 Day 1 |
| 17 | P2 | Frontend | Add nonce-based CSP, remove `unsafe-inline` | Phase 2 Day 3 |

All other items: PASS. No blockers for Phase 1 merge.

---

## 6. Commit Checklist

- [x] Days 8–21 each have a dedicated commit with a descriptive message
- [x] No secrets committed (`.env` excluded from git)
- [x] No `go.sum` mismatches (integration test deps use build tags, no new runtime deps)
- [x] All unit tests in `internal/` are in `*_test.go` files with proper package declarations
- [x] Integration tests use `//go:build integration` tag and skip without `TEST_DATABASE_URL`
- [x] k6 load test has documented thresholds and run instructions
- [x] Docker images use multi-stage builds and non-root users
- [x] Nginx CSP header present (with known `unsafe-inline` caveat tracked above)
- [x] Makefile has `help` target listing all public targets
