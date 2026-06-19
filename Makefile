.PHONY: up down reset build logs health seed psql test lint

COMPOSE := docker compose
SERVICE ?=

## up: Start all services (build images if needed)
up:
	$(COMPOSE) up -d --build

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

## health: Check health of all services
health:
	@echo "Checking service health..."
	@curl -sf http://localhost:8080/healthz && echo "  collector-service: PASS" || echo "  collector-service: FAIL"
	@curl -sf http://localhost:8081/healthz && echo "  query-api:         PASS" || echo "  query-api:         FAIL"
	@curl -sf http://localhost:3000/health  && echo "  opentrace-ui:      PASS" || echo "  opentrace-ui:      FAIL"

## seed: Send 1,000 sample log events to the collector
seed:
	go run ./scripts/seed/main.go

## psql: Open a psql shell in the postgres container
psql:
	$(COMPOSE) exec postgres psql -U $${POSTGRES_USER} -d $${POSTGRES_DB}

## test: Run all Go tests
test:
	go test ./... -race -timeout 60s

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## proto: Regenerate Protobuf code
proto:
	buf generate

## help: Show this help
help:
	@grep -E '^## ' Makefile | sed 's/## //'
