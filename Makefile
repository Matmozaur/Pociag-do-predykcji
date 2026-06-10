# ──────────────────────────────────────────────────────────────────────────────
# Pociag do Predykcji — Developer Makefile
#
# All developer tasks live here. Add per-service targets as services are built.
# Run `make help` to see all available targets.
#
# Prerequisites:
#   Docker          — https://docs.docker.com/get-docker/
#   Go 1.23+        — https://go.dev/dl/
#   uv              — winget install --id astral-sh.uv -e
#   gh copilot CLI  — gh extension install github/gh-copilot
# ──────────────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2}' | sort

# ── Infrastructure stack ──────────────────────────────────────────────────────

.PHONY: infra-up
infra-up: ## Start core infra (postgres + otel-collector)
	docker compose -f infra/docker-compose.yml up -d

.PHONY: infra-up-tracing
infra-up-tracing: ## Start infra + Jaeger tracing UI
	docker compose -f infra/docker-compose.yml --profile tracing up -d

.PHONY: infra-up-monitoring
infra-up-monitoring: ## Start infra + Prometheus + Grafana
	docker compose -f infra/docker-compose.yml --profile monitoring up -d

.PHONY: infra-up-airflow
infra-up-airflow: ## Start infra + Airflow
	docker compose -f infra/docker-compose.yml --profile airflow up -d

.PHONY: infra-up-all
infra-up-all: ## Start everything
	docker compose -f infra/docker-compose.yml --profile all up -d

.PHONY: infra-down
infra-down: ## Stop all infra containers (preserves volumes)
	docker compose -f infra/docker-compose.yml --profile all down

.PHONY: infra-down-volumes
infra-down-volumes: ## Stop all containers and remove volumes (DESTRUCTIVE)
	docker compose -f infra/docker-compose.yml --profile all down -v

.PHONY: infra-logs
infra-logs: ## Follow logs from all running infra containers
	docker compose -f infra/docker-compose.yml --profile all logs -f

# ── Database ──────────────────────────────────────────────────────────────────

DB_URL ?= postgres://pociag:pociag_dev_secret@localhost:5432/pociag?sslmode=disable

.PHONY: db-migrate-up
db-migrate-up: ## Apply all pending migrations
	docker run --rm --network host \
		-v $(PWD)/db/migrations:/migrations:ro \
		migrate/migrate:v4.18.1 \
		-path /migrations -database "$(DB_URL)" up

.PHONY: db-migrate-down
db-migrate-down: ## Roll back the last migration
	docker run --rm --network host \
		-v $(PWD)/db/migrations:/migrations:ro \
		migrate/migrate:v4.18.1 \
		-path /migrations -database "$(DB_URL)" down 1

.PHONY: db-migrate-status
db-migrate-status: ## Show current migration version
	docker run --rm --network host \
		-v $(PWD)/db/migrations:/migrations:ro \
		migrate/migrate:v4.18.1 \
		-path /migrations -database "$(DB_URL)" version

.PHONY: db-psql
db-psql: ## Open a psql shell to the local database
	docker compose -f infra/docker-compose.yml exec postgres \
		psql -U pociag -d pociag

# ── Per-service targets ───────────────────────────────────────────────────────
# Add targets here as services are scaffolded, e.g.:
#
# .PHONY: <service>-test
# <service>-test: ## Run tests for <service>
# 	cd services/go/<service> && go test -race ./...
#
# .PHONY: <service>-lint
# <service>-lint: ## Lint <service>
# 	cd services/go/<service> && golangci-lint run ./...

.PHONY: processor-format
processor-format: ## Format processor Python service
	$(MAKE) -C services/python/processor format

.PHONY: processor-test
processor-test: ## Run processor tests
	$(MAKE) -C services/python/processor test

.PHONY: processor-check
processor-check: ## Lint + typecheck + test processor
	$(MAKE) -C services/python/processor check

# ── GitHub Copilot CLI ────────────────────────────────────────────────────────

.PHONY: copilot-suggest
copilot-suggest: ## Interactive: ask Copilot CLI to suggest a shell command
	gh copilot suggest

.PHONY: copilot-explain
copilot-explain: ## Interactive: ask Copilot CLI to explain a command
	gh copilot explain

# ── Observability URLs ────────────────────────────────────────────────────────

.PHONY: urls
urls: ## Print local UI URLs
	@echo ""
	@echo "  Postgres        → localhost:5432  (pociag/pociag_dev_secret)"
	@echo "  Airflow UI      → http://localhost:8090  (profile: airflow)"
	@echo "  Jaeger UI       → http://localhost:16686 (profile: tracing)"
	@echo "  Prometheus      → http://localhost:9090  (profile: monitoring)"
	@echo "  Grafana         → http://localhost:3000  (profile: monitoring)"
	@echo ""
