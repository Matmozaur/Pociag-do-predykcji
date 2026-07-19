# AGENTS.md

Tool-agnostic guidance for AI coding agents (Copilot CLI, Copilot Chat, and others).
For VS Code Copilot Chat this complements `.github/copilot-instructions.md` and the
path-scoped rules in `.github/instructions/`.

## Project

**Pociag do Predykcji** — a train-delay data platform.

- **Go services** (`services/go/`): `collector` (PLK ingest → MinIO raw Parquet),
  `data-service` (read API over curated Postgres), `gateway` (frontend BFF).
- **Python** (`airflow/`): processing runs as an Airflow plugin (`pociag_processing`),
  not a standalone service.
- **Frontend** (`services/frontend/`): Next.js 15 + React 19 + TanStack Query.
- **Database**: PostgreSQL 16, migrations in `db/migrations/` (golang-migrate).

## Contract-first (non-negotiable)

1. Specs are the source of truth: `specs/openapi/`, `specs/asyncapi/`, `specs/schemas/`.
2. Do not add endpoints, fields, events, or schema attributes not defined in specs.
3. If code needs a contract change, update the spec first, then implement.
4. On doc conflicts, trust in order: `infra/docker-compose.yml` (runtime ports) →
   `.github/workflows/ci-cd.yaml` (CI checks) → `Makefile` (local commands).

## Build & test commands

Use the `Makefile` as the entrypoint (`make help` lists targets):

- Infra: `make infra-up`, `make infra-up-all`, `make infra-down`
- DB: `make db-migrate-up`, `make db-migrate-down`, `make db-migrate-status`
- Test: `make collector-test`, `make data-service-test`, `make gateway-test`
- Lint: `make collector-lint`, `make data-service-lint`, `make gateway-lint`
- Python: `uv run pytest airflow/tests -v`, `uv run ruff check`, `uv run mypy`

## Coding conventions (summary)

- **Go**: `context.Context` first param; `net/http` + `chi`; `pgx` parameterized SQL;
  OpenTelemetry spans on handlers/DB; wrap errors with `fmt.Errorf("...: %w", err)`.
- **Python**: `uv` envs; full type annotations, `mypy --strict` clean; idempotent DAGs;
  Airflow connections/hooks for external systems.
- **SQL**: paired `NNN_*.up.sql` / `.down.sql`; parameterized queries only.
- **Security**: never hardcode secrets; never log sensitive payloads; validate at
  system boundaries; keep code free of OWASP Top 10 issues.

## Shell environment

- Prefer the existing WSL terminal. Enter WSL once with a plain `wsl`, then
  `cd /mnt/c/Users/Admin/Documents/IT/Pociag-do-predykcji` and run commands directly.
- Do not use wrapper forms like `wsl bash -c "..."`, `wsl -e <cmd>`, or `wsl <command>`.
- Avoid PowerShell for repo tasks unless the task is explicitly Windows-specific.
