.PHONY: help docker-up docker-down docker-logs docker-clean shell build test test-unit lint fmt tidy swagger docker-up-prod docker-down-prod docker-logs-prod docker-clean-prod shell-prod

COMPOSE_DEV = docker compose -f docker-compose.yml
COMPOSE_PROD = docker compose -f docker-compose.prod.yml

help:
	@echo "Available commands:"
	@echo "  make docker-up    - Build and start app + MySQL + Redis in Docker"
	@echo "  make docker-down  - Stop Docker services"
	@echo "  make docker-logs  - Show app logs"
	@echo "  make docker-clean - Stop services and remove volumes"
	@echo "  make shell        - Open shell in app container"
	@echo "  make docker-up-prod    - Start prod compose (app + MySQL + Redis)"
	@echo "  make docker-down-prod  - Stop prod compose"
	@echo "  make docker-logs-prod  - Show prod app logs"
	@echo "  make docker-clean-prod - Stop prod compose and remove volumes"
	@echo "  make shell-prod        - Open shell in prod app container"
	@echo "  make build        - Build API binary in app container"
	@echo "  make test         - Run all tests (incl. MySQL integration) in app container"
	@echo "  make test-unit    - Run unit tests locally (integration tests are skipped)"
	@echo "  make lint         - Run go vet in app container"
	@echo "  make fmt          - Run gofmt in app container"
	@echo "  make tidy         - Run go mod tidy in app container"
	@echo "  make swagger      - Regenerate OpenAPI spec from handler annotations"

docker-up:
	@$(COMPOSE_DEV) up -d --build

docker-down:
	@$(COMPOSE_DEV) down

docker-logs:
	@$(COMPOSE_DEV) logs -f app

docker-clean:
	@$(COMPOSE_DEV) down -v

shell:
	@$(COMPOSE_DEV) exec app /bin/sh

docker-up-prod:
	@$(COMPOSE_PROD) up -d --build

docker-down-prod:
	@$(COMPOSE_PROD) down

docker-logs-prod:
	@$(COMPOSE_PROD) logs -f app

docker-clean-prod:
	@$(COMPOSE_PROD) down -v

shell-prod:
	@$(COMPOSE_PROD) exec app /bin/sh

build:
	@$(COMPOSE_DEV) run --rm --no-deps app go build -o /tmp/api ./cmd/api

test:
	@$(COMPOSE_DEV) run --rm app go test ./... -v -count=1

test-unit:
	@go test ./... -count=1

lint:
	@$(COMPOSE_DEV) run --rm --no-deps app go vet ./...

fmt:
	@$(COMPOSE_DEV) run --rm --no-deps app sh -c 'find . -name "*.go" -print0 | xargs -0 gofmt -w'

tidy:
	@$(COMPOSE_DEV) run --rm --no-deps app go mod tidy

swagger:
	@go run github.com/swaggo/swag/cmd/swag@v1.16.4 init -g server.go -d ./internal/api,./internal/domain -o ./internal/api/docs --outputTypes go,yaml
	@go run github.com/swaggo/swag/cmd/swag@v1.16.4 fmt -d ./internal/api
