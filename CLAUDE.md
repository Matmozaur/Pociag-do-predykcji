# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Layout

Monorepo. Four areas, each with its own toolchain:

| Area | Path | Notes |
|------|------|-------|
| Go services | `services/go/{collector,data-service,gateway}` + `services/go/shared` | Separate modules, `go 1.25.0`. Each: `cmd/` entrypoint, `internal/` packages. |
| Python / Airflow | `airflow/` — DAGs in `airflow/dags/`, transform logic in `airflow/plugins/pociag_processing/` | Processing runs **as an Airflow plugin, not a service** (`docs/decisions/003-processor-to-airflow-plugin.md`). |
| DB migrations | `db/migrations/` | golang-migrate, paired `NNN_name.up.sql` / `.down.sql`. |
| Frontend | `services/frontend/` | Next.js 15 App Router, React 19, Tailwind v4. Talks only to the gateway. |

Data flow: PLK API → collector → MinIO raw Parquet → `pociag_processing` (Airflow) → Postgres → data-service → gateway → frontend.

## Contract-first (non-negotiable)

Specs in `specs/openapi/`, `specs/asyncapi/`, `specs/schemas/` are the source of truth. Do not add
endpoints/fields/events/columns not in the specs — change the spec first. On doc conflicts trust:
`infra/docker-compose.yml` → `.github/workflows/ci-cd.yaml` → `Makefile`. Per-area rules live in
`.github/instructions/*.instructions.md`.

## Commands

`make help` lists targets. Run these from the repo root unless noted.

### Go (`services/go/<svc>`)

```bash
make collector-test        # cd services/go/collector && go test -race -v ./...   (also data-service-test, gateway-test)
make collector-lint        # cd ... && golangci-lint run ./...   (also data-service-lint, gateway-lint)
make collector-build       # swag init -g cmd/main.go -o internal/docs && go build -o /tmp/collector ./cmd
```

- Lint needs **golangci-lint v2** (config files are `services/go/<svc>/.golangci.yml`, `version: "2"`);
  CI installs it via `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`.
  Enabled linters: errcheck, govet, ineffassign, staticcheck, unused.
- `*-build` / `*-docs` need the `swag` CLI on PATH (`go install github.com/swaggo/swag/cmd/swag@latest`);
  it is not a module dependency. Not required for test/lint.
- Single test: `cd services/go/collector && go test -race -run TestName ./internal/service`.
- `services/go/shared` has no Make targets — `cd services/go/shared && go test -race ./...` directly.
- Integration tests use `testcontainers-go`, gated behind the `integration` build tag.

### Python / Airflow (run from `airflow/`)

```bash
cd airflow
uv sync --all-extras && uv pip install -e plugins   # first-time setup; plugin is a separate package (plugins/pyproject.toml)
uv run pytest tests/ -v                             # single: uv run pytest tests/test_pipelines_operations.py::test_name -v
uv run ruff check .                                 # line-length 100, target py312
uv run mypy plugins/pociag_processing               # strict; must be clean
```

- `tests/` are plain unit tests over pipeline functions and `repository.py` using `pytest-mock` —
  **there is no DagBag / DAG-import test and no local Airflow scheduler needed**. To eyeball a DAG,
  start Airflow (`make infra-up-airflow`, UI at http://localhost:8090) and check it parses there.
- No `conftest.py`; tests never import `airflow` runtime state.

### DB migrations

Migrations run in a Docker container (`migrate/migrate:v4.18.1`, `--network host`) against
`DB_URL` — default `postgres://pociag:pociag_dev_secret@127.0.0.1:5434/pociag?sslmode=disable`
(note port **5434**). Postgres must be up (`make infra-up`).

```bash
make db-migrate-up        # apply all pending
make db-migrate-down      # roll back exactly ONE
make db-migrate-status    # current version
make db-psql              # psql shell via docker compose exec
```

### Frontend (run from `services/frontend/`)

```bash
npm install
npm run dev              # next dev on :3000
npm run build            # next build — the ONLY build/lint gate; runs ESLint (eslint-config-next) as part of the build
npm start               # next start
```

There is **no `lint` or `test` npm script** and no standalone ESLint config file — linting only
happens inside `next build`. Type checking is via `tsc` through the build (`noEmit`, strict).

## Env vars

Copy `infra/.env.example` → `infra/.env` before `docker compose` / `make infra-*`. Keys:
`PLK_BASE_URL`, `PLK_API_KEY`, `POSTGRES_PASSWORD`, `MINIO_ROOT_USER`, `MINIO_ROOT_PASSWORD`,
`AIRFLOW_DB_PASSWORD`, `AIRFLOW__CORE__FERNET_KEY`, `AIRFLOW__WEBSERVER__SECRET_KEY`.

Running a Go service **outside** compose needs these (all via `os.LookupEnv`, no defaults — missing = startup error):

- collector: `HTTP_ADDR`, `DATABASE_DSN`, `PLK_BASE_URL`, `PLK_API_KEY`, `S3_ENDPOINT`, `S3_BUCKET`,
  `S3_ACCESS_KEY`, `S3_SECRET_KEY`; optional `S3_USE_PATH_STYLE=true`, `OTEL_EXPORTER_OTLP_ENDPOINT`.
- data-service: `HTTP_ADDR`, `DATABASE_DSN`; optional `OTEL_EXPORTER_OTLP_ENDPOINT`.
- gateway: `HTTP_ADDR`, `DATA_SERVICE_BASE_URL`; optional `OTEL_EXPORTER_OTLP_ENDPOINT`.
- frontend: `GATEWAY_URL` (default `http://localhost:8084`); `/bff/*` is rewritten to it.

Airflow reaches external systems via connections seeded in compose: `pociag_postgres`,
`pociag_collector`, `pociag_s3`. In code, use Airflow hooks — never inline clients with credentials.

## Runtime ports (host) — from `infra/docker-compose.yml`, which overrides README/architecture.md

| Service | Host port | Container |
|---------|-----------|-----------|
| Postgres (main) | **5434** | 5432 |
| Postgres (Airflow) | 5433 | 5432 |
| Collector | **8082** | 8081 |
| Data Service | 8083 | 8083 |
| Gateway (BFF) | **8084** | 8084 |
| Airflow UI | 8090 | 8080 |
| Jaeger 16686 · Prometheus 9090 · Grafana 3000 · MinIO 9000/9001 | as-is | |

`README.md` and `docs/architecture.md` say collector 8081 / gateway 8080 — stale for host access.

## Conventions (summary; full text in `.github/instructions/`)

- **Go**: `context.Context` first param on any I/O fn; `net/http` + `chi` only; `pgx/v5`
  parameterized queries only; wrap handlers/DB calls in `otel.Tracer("pociag.<svc>").Start(ctx, "<noun>.<verb>")`;
  wrap errors `fmt.Errorf("op: %w", err)`; config from `os.LookupEnv` only.
- **Python**: full type annotations, `mypy --strict` clean; `structlog`; DAGs idempotent and
  re-runnable for any execution date; query code in `pociag_processing/repository.py`.
- **SQL**: `.down` must fully reverse `.up`; add indexes for new FKs and common predicates;
  keep shapes aligned with `specs/schemas/`.
- **Frontend**: server data only through `@tanstack/react-query` hooks in `src/lib/` (no ad-hoc
  `fetch` in components); match `specs/openapi/gateway.yml`; handle loading + error states.

## Shell

Work inside WSL at `/mnt/c/Users/Admin/Documents/IT/Pociag-do-predykcji`; run commands directly,
not via `wsl bash -c "..."` wrappers.
