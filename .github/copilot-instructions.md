# Copilot Workspace Instructions

These instructions are for AI coding agents working in this repository.

## Start Here

- Read [README.md](../README.md) for local setup and entry commands.
- Read [docs/architecture.md](../docs/architecture.md) for system boundaries.
- Treat [specs/openapi/api.yml](../specs/openapi/api.yml) and [specs/pipelines.md](../specs/pipelines.md) as source of truth.

## Contract-First Rules

1. Specs first: do not add endpoints, fields, or schema attributes not defined in specs.
2. If implementation requires a contract change, update specs first, then code.
3. If docs disagree, trust these files in order:
   - [infra/docker-compose.yml](../infra/docker-compose.yml) for runtime ports/services
   - [.github/workflows/ci-cd.yaml](workflows/ci-cd.yaml) for CI build/lint/test behavior
   - [Makefile](../Makefile) for local command entrypoints

## Current Architecture Snapshot

- Ingestion: Airflow DAGs and the `pociag_processing` plugin ([airflow/](../airflow)) fetch the PLK API and write PostgreSQL.
- Serving: one Go read API, [services/go/api](../services/go/api).
- Frontend: [services/frontend](../services/frontend), talks only to the api.

## Coding Conventions

### Go

- Always pass context.Context as the first parameter.
- Use net/http and chi.
- Use pgx with parameterized SQL only.
- Instrument handlers and DB operations with OpenTelemetry.
- Wrap errors with context using fmt.Errorf("...: %w", err).

### Python (Airflow + plugin)

- Use uv-managed environments.
- Keep type annotations throughout; maintain mypy cleanliness.
- Keep DAGs idempotent and use Airflow connections/hooks for external systems.
- Never hardcode credentials; use env vars or Airflow connections.

### Database and Security

- Migration naming: db/migrations/NNN_description.up.sql and .down.sql.
- Use parameterized SQL only; never concatenate untrusted values.
- Do not log secrets or sensitive payloads.

## Command Surface

- Preferred local entrypoint: [Makefile](../Makefile).
- Useful targets:
  - up, up-core, down, reset
  - db-migrate-up, db-migrate-down, db-migrate-status
  - test, api-test, api-lint, airflow-test, airflow-lint, frontend-build
- CI reference for exact checks: [.github/workflows/ci-cd.yaml](workflows/ci-cd.yaml).

## Shell Environment

- First shell action should use the existing WSL terminal session (or enter WSL with plain wsl once).
- After entering WSL, switch to /mnt/c/Users/Admin/Documents/IT/Pociag-do-predykcji and run commands there.
- Do not use wrapper forms such as wsl bash -c or wsl -e for normal execution.
- Avoid PowerShell for repo tasks unless the task is explicitly Windows-specific.

## Additional References

- Copilot CLI usage patterns: [docs/copilot-cli.md](../docs/copilot-cli.md)
