.PHONY: up up-dev down reset build logs health seed psql \
        test test-integration lint proto \
        web-install web-dev web-build web-typecheck

COMPOSE     := docker compose
COMPOSE_DEV := docker compose -f docker-compose.yml -f docker-compose.dev.yml
SERVICE     ?=

# ---------------------------------------------------------------------------
# Stack lifecycle
# ---------------------------------------------------------------------------

## up: Start all services in production config (internal ports only)
up:
	$(COMPOSE) up -d --build

## up-dev: Start all services with host ports exposed for local tooling
up-dev:
	$(COMPOSE_DEV) up -d --build

## down: Stop all services (preserve volumes)
down:
	$(COMPOSE) down

## reset: Destroy all services and volumes (DATA LOSS — prompts for confirmation)
reset:
	@echo "WARNING: This will destroy all data volumes. Press Ctrl+C to cancel, Enter to continue."
	@read confirm
	$(COMPOSE) down -v

## build: Force rebuild all images without cache
build:
	$(COMPOSE) build --no-cache

## logs: Tail logs for a service (usage: make logs service=collector-service)
logs:
	$(COMPOSE) logs -f $(service)

# ---------------------------------------------------------------------------
# Health and diagnostics
# ---------------------------------------------------------------------------

## health: Check health of all services
health:
	@echo "Checking service health..."
	@curl -sf http://localhost:8080/healthz && echo "  collector-service: PASS" || echo "  collector-service: FAIL"
	@curl -sf http://localhost:8081/healthz && echo "  query-api:         PASS" || echo "  query-api:         FAIL"
	@curl -sf http://localhost:3000/health  && echo "  opentrace-ui:      PASS" || echo "  opentrace-ui:      FAIL"

## psql: Open a psql shell in the postgres container
psql:
	$(COMPOSE) exec postgres psql -U $${POSTGRES_USER} -d $${POSTGRES_DB}

# ---------------------------------------------------------------------------
# Go backend
# ---------------------------------------------------------------------------

## seed: Send 1,000 sample log events to the collector
seed:
	go run ./scripts/seed/main.go

## test: Run all Go unit tests with race detector
test:
	go test ./... -race -timeout 60s

## test-integration: Run integration tests (requires TEST_DATABASE_URL)
test-integration:
	go test -tags=integration ./... -race -timeout 120s

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## proto: Regenerate Protobuf code
proto:
	buf generate

# ---------------------------------------------------------------------------
# React frontend
# ---------------------------------------------------------------------------

## web-install: Install npm dependencies
web-install:
	cd web && npm ci

## web-dev: Start Vite dev server (proxies /api to localhost:8081)
web-dev:
	cd web && npm run dev

## web-build: Production build of the React app
web-build:
	cd web && npm run build

## web-typecheck: TypeScript type-check without emitting files
web-typecheck:
	cd web && npm run typecheck

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
