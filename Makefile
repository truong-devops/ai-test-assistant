COMPOSE := docker compose -f infra/compose/docker-compose.yml

.PHONY: dev-up dev-down migrate-up migrate-down test test-integration lint build smoke sample-test sandbox-build sandbox-test frontend-install frontend-typecheck frontend-build

dev-up: sandbox-build
	$(COMPOSE) up --build -d

sandbox-build:
	$(COMPOSE) build sandbox

sandbox-test: sandbox-build
	./scripts/sandbox-smoke.sh
	cd backend && RUN_SANDBOX_TESTS=1 SANDBOX_TEST_IMAGE="$${SANDBOX_IMAGE:-ai-test-assistant-sandbox:phase7}" go test -count=1 -tags=sandbox ./internal/validation ./internal/repair

dev-down:
	$(COMPOSE) down --remove-orphans

migrate-up:
	$(COMPOSE) run --rm migrate

migrate-down:
	$(COMPOSE) run --rm migrate-down

test: frontend-typecheck
	cd backend && go test ./...
	cd examples/go-microservices && go test ./...

test-integration:
	cd backend && TEST_DATABASE_URL="$${TEST_DATABASE_URL}" go test -p 1 -tags=integration ./internal/...

lint:
	cd backend && go vet ./...
	cd examples/go-microservices && go vet ./...

build: frontend-build
	cd backend && go build ./cmd/api ./cmd/worker

frontend-install:
	cd frontend && npm ci --ignore-scripts

frontend-typecheck: frontend/node_modules
	cd frontend && npm run typecheck

frontend-build: frontend/node_modules
	cd frontend && npm run build

frontend/node_modules: frontend/package.json frontend/package-lock.json
	cd frontend && npm ci --ignore-scripts

smoke:
	./scripts/smoke.sh

sample-test:
	cd examples/go-microservices && go test ./...
