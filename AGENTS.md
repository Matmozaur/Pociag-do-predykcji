# AGENTS.md

Tool-agnostic guidance for AI coding agents (Copilot CLI, Copilot Chat, and others).
For VS Code Copilot Chat this complements `.github/copilot-instructions.md` and the
path-scoped rules in `.github/instructions/`.

## Project

**Pociag do Predykcji** — a train-delay data platform (`docs/architecture.md`).

- **Ingestion** (`airflow/`): DAGs + the `pociag_processing` plugin fetch the PLK Open
  Data API and upsert PostgreSQL.
- **API** (`services/go/api/`): one Go read service shaped for the frontend.
- **Frontend** (`services/frontend/`): Next.js 15 + React 19 + TanStack Query.
- **Database**: PostgreSQL 16, migrations in `db/migrations/` (golang-migrate).

## Contract-first (non-negotiable)

1. Specs are the source of truth: `specs/openapi/api.yml`, `specs/pipelines.md`.
2. Do not add endpoints, fields, or schema attributes not defined in specs.
3. If code needs a contract change, update the spec first, then implement.
4. On doc conflicts, trust in order: `infra/docker-compose.yml` (runtime ports) →
   `.github/workflows/ci-cd.yaml` (CI checks) → `Makefile` (local commands).

## Build & test commands

Use the `Makefile` as the entrypoint (`make help` lists targets):

- Stack: `make up`, `make up-core`, `make down`, `make reset` (deletes data)
- DB: `make db-migrate-up`, `make db-migrate-down`, `make db-migrate-status`
- Checks: `make test` (= `api-test api-lint airflow-test airflow-lint frontend-build`)

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
