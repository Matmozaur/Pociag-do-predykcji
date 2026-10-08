# Pociag do Predykcji — developer tasks. Run `make help` for the list.
# Prerequisites: Docker, Go 1.25+, uv, Node 22.

COMPOSE := docker compose -f infra/docker-compose.yml
DB_URL ?= postgres://pociag:pociag_dev_secret@127.0.0.1:5434/pociag?sslmode=disable
MIGRATE := docker run --rm --network host -v $(PWD)/db/migrations:/migrations:ro \
	migrate/migrate:v4.18.1 -path /migrations -database "$(DB_URL)"
# golangci-lint from PATH, else from $(go env GOPATH)/bin (where `go install` puts it).
GOLANGCI_LINT ?= $(or $(shell command -v golangci-lint 2>/dev/null),$(shell go env GOPATH)/bin/golangci-lint)

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ── Stack ─────────────────────────────────────────────────────────────────────

.PHONY: up up-core down reset logs
up: ## Start everything (Postgres, api, Airflow, frontend, observability), rebuilding images
	$(COMPOSE) --profile all up -d --build

up-core: ## Start Postgres, migrations and the api only
	$(COMPOSE) up -d --build

down: ## Stop all containers (keeps data)
	$(COMPOSE) --profile all down

reset: ## Stop all containers and DELETE all data volumes
	$(COMPOSE) --profile all down -v --remove-orphans

logs: ## Follow logs of all containers
	$(COMPOSE) --profile all logs -f

# ── Database ──────────────────────────────────────────────────────────────────

.PHONY: db-migrate-up db-migrate-down db-migrate-status db-psql
db-migrate-up: ## Apply pending migrations (compose also runs them on start)
	$(MIGRATE) up

db-migrate-down: ## Roll back the last migration
	$(MIGRATE) down 1

db-migrate-status: ## Show the current migration version
	$(MIGRATE) version

db-psql: ## Open a psql shell on the curated database
	$(COMPOSE) exec postgres psql -U pociag -d pociag

# ── Checks ────────────────────────────────────────────────────────────────────

.PHONY: test api-test api-lint airflow-test airflow-lint frontend-build
test: api-test api-lint airflow-test airflow-lint frontend-build ## Run every check CI runs, plus the frontend build

api-test: ## Go tests (set POCIAG_TEST_DATABASE_URL to include the SQL tests)
	cd services/go/api && go test -race ./...

api-lint: ## golangci-lint (v2) on the api
	cd services/go/api && $(GOLANGCI_LINT) run ./...

airflow-test: ## pytest (set POCIAG_TEST_DATABASE_URL to include the SQL tests)
	cd airflow && uv sync --all-extras -q && uv pip install -q -e plugins && uv run pytest tests -q

airflow-lint: ## ruff + mypy --strict on the Airflow code
	cd airflow && uv run ruff check . && uv run mypy plugins/pociag_processing dags

frontend-build: ## Next.js build (type check + lint)
	cd services/frontend && npm run build

# ── URLs ──────────────────────────────────────────────────────────────────────

.PHONY: urls
urls: ## Print local URLs
	@echo "  Frontend    http://localhost:3100"
	@echo "  API         http://localhost:8080/api/v1/dashboard/overview"
	@echo "  Airflow     http://localhost:8090 (admin/admin)"
	@echo "  Postgres    localhost:5434 (pociag)"
	@echo "  Jaeger      http://localhost:16686"
	@echo "  Prometheus  http://localhost:9090   Grafana http://localhost:3001"
