COMPOSE := docker compose -f infra/compose/docker-compose.yml

.PHONY: dev-up dev-down migrate-up migrate-down test test-integration lint build smoke sample-test

dev-up:
	$(COMPOSE) up --build -d

dev-down:
	$(COMPOSE) down --remove-orphans

migrate-up:
	$(COMPOSE) run --rm migrate

migrate-down:
	$(COMPOSE) run --rm migrate-down

test:
	cd backend && go test ./...
	cd examples/go-microservices && go test ./...

test-integration:
	cd backend && TEST_DATABASE_URL="$${TEST_DATABASE_URL}" go test -p 1 -tags=integration ./internal/...

lint:
	cd backend && go vet ./...
	cd examples/go-microservices && go vet ./...

build:
	cd backend && go build ./cmd/api ./cmd/worker

smoke:
	./scripts/smoke.sh

sample-test:
	cd examples/go-microservices && go test ./...
