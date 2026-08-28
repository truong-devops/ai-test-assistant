COMPOSE := docker compose -f infra/compose/docker-compose.yml

.PHONY: dev-up dev-down migrate-up migrate-down test test-integration lint build smoke sample-test sandbox-build sandbox-test sandbox-security-check frontend-install frontend-typecheck frontend-build evaluate evaluate-import prod-config prod-up prod-migrate backup restore

EVALUATION_DATASET ?= evaluation/datasets/controlled-v1.json
EVALUATION_OUTPUT ?= evaluation/results/controlled-v1
EVALUATION_DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/ai_test_assistant?sslmode=disable

dev-up: sandbox-build
	$(COMPOSE) up --build -d

sandbox-build:
	$(COMPOSE) build sandbox

sandbox-test: sandbox-build
	./scripts/sandbox-smoke.sh
	cd backend && RUN_SANDBOX_TESTS=1 SANDBOX_TEST_IMAGE="$${SANDBOX_IMAGE:-ai-test-assistant-sandbox:phase7}" go test -count=1 -tags=sandbox ./internal/validation ./internal/repair

sandbox-security-check:
	./scripts/sandbox-security-check.sh

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
	cd backend && go build ./cmd/api ./cmd/worker ./cmd/evaluate ./cmd/healthcheck

prod-config:
	docker compose --env-file "$${ENV_FILE:-.env.production}" -f infra/compose/docker-compose.prod.yml config --quiet

prod-up:
	./scripts/prod-up.sh

prod-migrate:
	./scripts/prod-migrate.sh

backup:
	./scripts/backup.sh

restore:
	./scripts/restore.sh

evaluate:
	cd backend && go run ./cmd/evaluate -dataset ../$(EVALUATION_DATASET) -out ../$(EVALUATION_OUTPUT)

evaluate-import:
	cd backend && go run ./cmd/evaluate -dataset ../$(EVALUATION_DATASET) -out ../$(EVALUATION_OUTPUT) -database-url "$(EVALUATION_DATABASE_URL)"

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
