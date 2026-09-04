COMPOSE := docker compose -f infra/compose/docker-compose.yml
ENV_FILE ?= .env.production
COMPOSE_FILE ?= infra/compose/docker-compose.prod.yml
LOG_TAIL ?= 100
READY_URL ?= http://127.0.0.1:8080/ready
PROD_COMPOSE = docker compose --env-file "$(ENV_FILE)" -f "$(COMPOSE_FILE)"

.PHONY: dev-up dev-down migrate-up migrate-down test test-integration lint build smoke sample-test sandbox-build sandbox-test sandbox-security-check frontend-install frontend-typecheck frontend-build evaluate evaluate-import rebuild rebuild-be rebuild-fe rebuild-worker rebuild-all prod-help prod-config prod-up prod-migrate prod-gemini-setup prod-gemini-up prod-gemini-smoke prod-rebuild-api prod-rebuild-worker prod-rebuild-backend prod-rebuild-frontend prod-rebuild-app prod-rebuild-all prod-worker-restart prod-status prod-logs prod-worker-logs prod-worker-logs-follow backup restore

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

prod-help:
	@echo "Production commands:"
	@echo "  make rebuild                  Rebuild API + frontend and check /ready"
	@echo "  make rebuild-be               Rebuild backend API and worker, then migrate"
	@echo "  make rebuild-fe               Rebuild only the Next.js frontend"
	@echo "  make rebuild-worker           Rebuild only the worker/AI service"
	@echo "  make rebuild-all              Full rebuild including the sandbox"
	@echo "  make prod-up                  Build and start the complete production stack"
	@echo "  make prod-gemini-setup        Store the Gemini key and update .env.production"
	@echo "  make prod-gemini-up           Configure Gemini, rebuild and restart the worker"
	@echo "  make prod-gemini-smoke        Call Gemini directly without creating an analysis"
	@echo "  make prod-rebuild-api         Rebuild only the API after API-only changes"
	@echo "  make prod-rebuild-worker      Rebuild only the worker after worker/LLM changes"
	@echo "  make prod-rebuild-backend     Run migrations and rebuild API plus worker"
	@echo "  make prod-rebuild-frontend    Rebuild only the Next.js frontend"
	@echo "  make prod-rebuild-app         Rebuild API, worker and frontend"
	@echo "  make prod-rebuild-all         Full rebuild including the sandbox"
	@echo "  make prod-worker-restart      Recreate the worker without rebuilding its image"
	@echo "  make prod-status              Show production container status"
	@echo "  make prod-worker-logs         Show the latest worker logs"
	@echo "  make prod-worker-logs-follow  Follow worker logs; stop with Ctrl+C"
	@echo "  make prod-logs                Show API, worker and frontend logs"
	@echo "Options: ENV_FILE=.env.production COMPOSE_FILE=... READY_URL=... LOG_TAIL=100"

prod-config:
	test -f "$(ENV_FILE)" || { echo "missing $(ENV_FILE); copy .env.production.example first" >&2; exit 1; }
	$(PROD_COMPOSE) config --quiet

prod-up:
	ENV_FILE="$(ENV_FILE)" COMPOSE_FILE="$(COMPOSE_FILE)" ./scripts/prod-up.sh

prod-migrate:
	ENV_FILE="$(ENV_FILE)" COMPOSE_FILE="$(COMPOSE_FILE)" ./scripts/prod-migrate.sh

prod-gemini-setup:
	ENV_FILE="$(ENV_FILE)" ./scripts/configure-gemini.sh
	$(PROD_COMPOSE) config --quiet

prod-gemini-up:
	ENV_FILE="$(ENV_FILE)" ./scripts/configure-gemini.sh
	$(PROD_COMPOSE) config --quiet
	$(PROD_COMPOSE) up -d --build --no-deps --wait --wait-timeout 120 worker
	$(PROD_COMPOSE) ps worker

prod-gemini-smoke:
	ENV_FILE="$(ENV_FILE)" ./scripts/gemini-smoke.sh

prod-rebuild-api: prod-config
	$(PROD_COMPOSE) up -d --build --no-deps --wait --wait-timeout 120 api
	$(PROD_COMPOSE) ps api

prod-rebuild-worker: prod-config
	$(PROD_COMPOSE) up -d --build --no-deps --wait --wait-timeout 120 worker
	$(PROD_COMPOSE) ps worker

prod-rebuild-backend: prod-config
	$(PROD_COMPOSE) build api worker
	$(PROD_COMPOSE) --profile tools run --rm migrate
	$(PROD_COMPOSE) up -d --no-deps --wait --wait-timeout 120 api worker
	$(PROD_COMPOSE) ps api worker

prod-rebuild-frontend: prod-config
	$(PROD_COMPOSE) up -d --build --no-deps --wait --wait-timeout 180 frontend
	$(PROD_COMPOSE) ps frontend

prod-rebuild-app: prod-config
	$(PROD_COMPOSE) build api worker frontend
	$(PROD_COMPOSE) --profile tools run --rm migrate
	$(PROD_COMPOSE) up -d --wait --wait-timeout 180 postgres api worker frontend
	$(PROD_COMPOSE) ps

prod-rebuild-all: prod-up

rebuild: prod-config
	$(PROD_COMPOSE) up -d --build --wait --wait-timeout 180 api frontend
	$(PROD_COMPOSE) ps api frontend
	curl --fail --silent --show-error "$(READY_URL)"
	@echo

rebuild-be: prod-rebuild-backend

rebuild-fe: prod-rebuild-frontend

rebuild-worker: prod-rebuild-worker

rebuild-all: prod-rebuild-all

prod-worker-restart: prod-config
	$(PROD_COMPOSE) up -d --no-deps --force-recreate --wait --wait-timeout 120 worker
	$(PROD_COMPOSE) ps worker

prod-status: prod-config
	$(PROD_COMPOSE) ps

prod-logs: prod-config
	$(PROD_COMPOSE) logs --tail="$(LOG_TAIL)" api worker frontend

prod-worker-logs: prod-config
	$(PROD_COMPOSE) logs --tail="$(LOG_TAIL)" worker

prod-worker-logs-follow: prod-config
	$(PROD_COMPOSE) logs --tail="$(LOG_TAIL)" --follow worker

backup:
	ENV_FILE="$(ENV_FILE)" COMPOSE_FILE="$(COMPOSE_FILE)" ./scripts/backup.sh

restore:
	ENV_FILE="$(ENV_FILE)" COMPOSE_FILE="$(COMPOSE_FILE)" ./scripts/restore.sh

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
