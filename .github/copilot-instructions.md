# Copilot Workspace Instructions

## Project Overview

**Pociag do Predykcji** ("Train to Prediction") is a data monitoring and prediction platform.
The architecture, services, and data models are defined incrementally through specs — do not
assume or invent components that are not described in `specs/`.

## Spec-Driven Development Workflow

> **Always consult `specs/` before generating any code.**

1. **Spec first** — API contracts, data schemas, and event definitions live in `specs/`.
   Code must match the spec exactly. If a spec is ambiguous, flag it rather than guessing.
2. **Implement from spec** — use the `#implement-from-spec` prompt to scaffold or fill in
   a service from an OpenAPI/AsyncAPI spec.
3. **Update spec with code** — if implementation reveals a necessary spec change, update the
   spec file first, then update the code.
4. **No accidental contracts** — do not add endpoints, fields, or events that are not in the spec.

## Repository Layout

```
.github/
  copilot-instructions.md   ← you are here
  prompts/                  ← reusable Copilot prompt files
specs/
  openapi/                  ← OpenAPI 3.1 service API contracts
  asyncapi/                 ← AsyncAPI event/message contracts (if needed)
  schemas/                  ← JSON Schema domain entity definitions
services/
  go/
    <service-name>/         ← microservice (one per subdirectory)
airflow/
  dags/                     ← Airflow DAG definitions
  plugins/                  ← custom Airflow operators/hooks
db/
  migrations/               ← numbered SQL migration files
infra/
  docker-compose.yml        ← infrastructure services only (db, otel, etc.)
  otel-collector-config.yml ← OpenTelemetry Collector config
docs/
  decisions/                ← Architecture Decision Records (ADRs)
  copilot-cli.md
Makefile                    ← all developer tasks
```

## Tech Stack

| Layer | Technology |
|---|---|
| Go services | Go 1.23+ |
| Python services | Python 3.12+, FastAPI |
| Orchestration | Apache Airflow 2.9+ |
| Database | PostgreSQL 16 |
| Migrations | golang-migrate (SQL files) |
| Tracing | OpenTelemetry → Jaeger |
| Metrics | OpenTelemetry → Prometheus |
| Logging | Structured JSON (zap in Go, structlog in Python) |
| Container runtime | Docker Compose (local), Kubernetes (prod) |

## Go Conventions

- Module path: `github.com/pociag-do-predykcji/services/go/<service-name>`
- Use **`internal/`** for all non-exported packages; `cmd/` for entrypoints.
- Configuration via environment variables only; use a `config` package that reads with `os.LookupEnv`.
- **Always** propagate `context.Context` as the first parameter.
- Use `pgx/v5` directly (no ORM) for database access; queries live in `internal/repository/`.
- HTTP server: standard `net/http` with `chi` router.
- **Tracing**: instrument every HTTP handler and DB call with `go.opentelemetry.io/otel`.
- Error wrapping: `fmt.Errorf("operation: %w", err)` — never swallow errors.
- Tests: `_test.go` files alongside the code; use `testcontainers-go` for DB integration tests.
- Linting: `golangci-lint` with `.golangci.yml` at service root.

```go
// Example handler signature
func (h *Handler) HandleFetch(w http.ResponseWriter, r *http.Request) {
    ctx, span := otel.Tracer("pociag.<service>").Start(r.Context(), "<resource>.fetch")
    defer span.End()
    // ...
}
```

## Python Conventions

- Package manager: **uv**; `pyproject.toml` (PEP 621) at service root.
- Source layout: `src/<package>/` (import-mode = importlib).
- HTTP framework: **FastAPI** with async handlers.
- DB access: **asyncpg** + raw SQL; queries in `src/<pkg>/repository.py`.
- **Tracing**: `opentelemetry-sdk`, `opentelemetry-instrumentation-fastapi`.
- Logging: `structlog` configured to emit JSON.
- Type annotations required everywhere; validated with `mypy --strict`.
- Tests: **pytest** + `pytest-asyncio`; use `testcontainers` for Postgres integration tests.
- Linting: `ruff` for lint + format.

```python
# Example endpoint signature
@router.get("/<resources>/{id}")
async def get_resource(
    id: int,
    db: AsyncConnection = Depends(get_db),
    tracer: Tracer = Depends(get_tracer),
) -> ResourceResponse:
    with tracer.start_as_current_span("<resource>.fetch"):
        ...
```

## Airflow DAG Conventions

- All DAGs use the **`@dag` decorator** (TaskFlow API).
- Use `@task` for Python callables; use `DockerOperator` or `KubernetesPodOperator` for heavy jobs.
- DAG IDs: `snake_case`, descriptive, e.g. `ingest_source_daily`.
- Schedule via cron string or `timedelta`; always set `catchup=False` unless backfill is intentional.
- Connections stored in Airflow Connections, never hardcoded.
- DAGs must be **idempotent** — re-running a DAG for the same date must be safe.
- Always define `default_args` with `retries=2` and `retry_delay=timedelta(minutes=5)`.

```python
# DAG skeleton
from airflow.decorators import dag, task
from datetime import datetime, timedelta

@dag(
    dag_id="<dag_id>",
    schedule="@daily",
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args={"retries": 2, "retry_delay": timedelta(minutes=5)},
    tags=["pociag"],
)
def my_dag():
    @task
    def extract() -> dict: ...

    @task
    def load(data: dict) -> None: ...

    load(extract())
```

## Database Conventions

- Migration files: `db/migrations/<NNN>_<description>.{up,down}.sql` (zero-padded 3 digits).
- Schema naming: `snake_case` for tables and columns.
- Every table must have `id BIGSERIAL PRIMARY KEY`, `created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`, `updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()`.
- Indexes: create explicitly in migrations, not via ORM magic.
- Never `DROP` columns in a migration — add new columns and deprecate old ones.
- Use transactions in migrations; wrap each migration in `BEGIN; ... COMMIT;`.

## Observability

### Tracing
- Service names follow pattern: `pociag.<service>` (e.g. `pociag.ingestor`, `pociag.api`).
- Span names: `<noun>.<verb>` (e.g. `record.fetch`, `job.run`).
- Add span attributes for key business dimensions (record ID, source name, model version, etc.).
- All inter-service calls must propagate W3C TraceContext headers.

### Metrics
- Expose a `/metrics` endpoint (Prometheus scrape) on each service.
- Standard metrics per service: request count, request duration (histogram), active connections.

### Logging
- Emit structured JSON logs with fields: `level`, `ts`, `service`, `trace_id`, `span_id`, `msg`.
- Never log sensitive data (credentials, PII).

## Testing Guidelines

When asked to write tests:
1. Write unit tests first, mock external dependencies.
2. Write integration tests with real DB using testcontainers.
3. Aim for ≥ 80% coverage on business logic packages.
4. Test names: `Test<Function>_<Scenario>_<ExpectedBehavior>`.

## Security Guidelines

- Never hardcode secrets; use environment variables or secret managers.
- Validate all external input at service boundaries.
- Use parameterized queries — never string-concatenate SQL.
- Dependencies: run `govulncheck` (Go) and `pip audit` / `uv pip audit` (Python) in CI.

## GitHub Copilot CLI

Use `gh copilot` for shell command assistance during development:

```bash
# Suggest a command to accomplish a goal
gh copilot suggest "run only the <service> tests with race detection"

# Explain an unfamiliar command
gh copilot explain "docker compose --profile tracing up -d"
```

Alias recommendations (add to shell profile):
```bash
alias gcs='gh copilot suggest'
alias gce='gh copilot explain'
```

See `docs/copilot-cli.md` for detailed usage patterns.
