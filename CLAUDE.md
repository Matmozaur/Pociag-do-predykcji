# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Per-subproject guidance lives in nested `CLAUDE.md` files — read the one for the area you're touching:
`services/go/CLAUDE.md` (+ `collector/`, `data-service/`, `gateway/`, `shared/`), `airflow/CLAUDE.md`,
`db/migrations/CLAUDE.md`, `services/frontend/CLAUDE.md`.

## What this is

**Pociag do Predykcji** — ingests Polish railway data from the PLK Open Data API, lands it as raw
Parquet, transforms it into curated PostgreSQL tables, and serves it to a web frontend via a BFF.

## Architecture

ELT with read/write separation:

```
PLK API → collector (Go) → MinIO raw Parquet → pociag_processing (Airflow plugin) → PostgreSQL
                                                                                        ↓
                                        frontend (Next.js) ← gateway BFF (Go) ← data-service (Go)
```

| Component | Path | Role |
|-----------|------|------|
| collector | `services/go/collector` | PLK API pages → Parquet in MinIO `s3://pociag-lake/raw/`; writes `ingestion_run` rows |
| pociag_processing | `airflow/plugins/pociag_processing` | Reads raw Parquet, dedups/normalizes/enriches, upserts curated tables. **Runs inside Airflow, not as a service** (`docs/decisions/003-processor-to-airflow-plugin.md`) |
| data-service | `services/go/data-service` | Domain read API over curated PostgreSQL |
| gateway | `services/go/gateway` | Frontend-facing facade over data-service; CORS, cache, rate limit |
| frontend | `services/frontend` | Next.js 15; talks only to the gateway |

Four pipelines, one DAG each: dictionaries (weekly → `stations`, `carriers`, …), schedules
(weekly → `routes`, `route_stops`), operations (daily → `train_operations`), disruptions
(daily → `disruptions`).

## How the pieces talk

- **Airflow → collector**: HTTP `POST /api/v1/fetch/{pipeline}` via the `pociag_collector` Airflow
  connection. Airflow then calls the plugin's `process_*` functions in-process.
- **Plugin → MinIO / Postgres**: `pociag_s3` and `pociag_postgres` Airflow connections (hooks only).
- **frontend → gateway → data-service**: HTTP. The frontend proxies `/bff/*` to the gateway
  (`GATEWAY_URL`, default `http://localhost:8084`); the gateway reads `DATA_SERVICE_BASE_URL`.
- **Internal events**: PostgreSQL `LISTEN/NOTIFY` — `pociag.raw.*_fetched` (collector after landing),
  `pociag.data.*_ingested` (plugin after upsert).
- Both data-service and the plugin read the **same curated tables**; migrations must keep both working.

## Contract-first (non-negotiable)

Specs in `specs/openapi/`, `specs/asyncapi/`, `specs/schemas/` are the source of truth. Do not add
endpoints / fields / events / columns not in the specs — change the spec first, then the code.
On doc conflicts trust: `infra/docker-compose.yml` → `.github/workflows/ci-cd.yaml` → `Makefile`.
Full per-area rules are also in `.github/instructions/*.instructions.md` (mirror of the nested files).

## Scope discipline — keep the diff small

The change you ship should be the smallest one that does the task. Fewer touched files = faster
review, easier revert, cleaner history.

- **Out-of-scope findings → issue, not inline fix.** If you spot a bug or needed improvement
  unrelated to the current task, do **not** fix it inline. Run `gh issue create` describing the
  finding, its location (`file:line`), and why it matters, then continue what you were doing.
  Mention the issue number in your summary.
- **No drive-by edits.** Don't reformat, rename, re-order imports, "tidy" comments, bump
  dependencies, or restructure code you're only reading. If a formatter/linter rewrites files you
  didn't touch, revert those hunks before committing.
- **Match the surrounding code.** Follow the existing patterns, naming, and structure of the file
  you're in rather than introducing a new style — even one you'd prefer.
- **Don't widen the interface.** No new endpoints / fields / events / columns / config keys / CLI
  flags / exported functions beyond what the task needs (and the specs allow — see Contract-first).
- **Ask before large refactors.** If the task seems to require touching many files or moving code
  across modules, stop and confirm the approach with the user first.
- **Leave TODOs and dead code alone** unless removing them *is* the task. File an issue instead.
- **Tests and docs for what you changed**, not a sweep of pre-existing gaps.

## Repo-wide workflow

```bash
cp infra/.env.example infra/.env    # fill in real values first (see below)
make infra-up        # postgres + otel-collector  (profiles: tracing, monitoring, airflow, all)
make db-migrate-up   # apply migrations (see db/migrations/CLAUDE.md)
make infra-up-all    # everything, incl. Airflow UI :8090
make help            # all targets
```

`infra/.env` keys: `PLK_BASE_URL`, `PLK_API_KEY`, `POSTGRES_PASSWORD`, `MINIO_ROOT_USER`,
`MINIO_ROOT_PASSWORD`, `AIRFLOW_DB_PASSWORD`, `AIRFLOW__CORE__FERNET_KEY`,
`AIRFLOW__WEBSERVER__SECRET_KEY`.

### Runtime ports (host) — from `infra/docker-compose.yml`, which overrides README / architecture.md

| | Host | | Host |
|---|---|---|---|
| Postgres (main) | **5434** | Airflow Postgres | 5433 |
| collector | **8082** | data-service | 8083 |
| gateway (BFF) | **8084** | Airflow UI | 8090 |
| MinIO API / console | 9000 / 9001 | Jaeger / Prometheus / Grafana | 16686 / 9090 / 3000 |

`README.md` and `docs/architecture.md` say collector 8081 / gateway 8080 — stale for host access.

### Branch & commit conventions (observed)

- Branches: `feature/<slug>`, `fix/<slug>`, `chore/<slug>` (kebab-case). Some older ones are bare
  (`gateway_v1`, `basic-fe`) — prefer the prefixed form.
- Commits: short capitalized imperative subject, no Conventional-Commits prefix
  (`Add stations on the map`, `Fix ...`, `Update ...`, `Remove ...`). Keep them scoped.
- Merge to `main` via GitHub PR ("Merge pull request #N from Matmozaur/<branch>").
- CI (`.github/workflows/ci-cd.yaml`) runs on every branch push: Go test + lint per module,
  and ruff / mypy / pytest for `airflow/`. Build & deploy stages are placeholders.

## Cross-cutting conventions

- **No hardcoded secrets** — env vars (Go: `os.LookupEnv` only) or Airflow connections.
- **Tracing everywhere**: service name `pociag.<service>`, span name `<noun>.<verb>`, W3C
  TraceContext propagation across HTTP. OTLP gRPC to `OTEL_EXPORTER_OTLP_ENDPOINT` (:4317).
- **Structured JSON logs**: `zap` in Go, `structlog`/`logging` in Python. Never log secrets or
  raw sensitive payloads.
- **Parameterized SQL only**, on both sides (Go and Python). Never concatenate untrusted values.

## Shell

Work inside WSL at `/mnt/c/Users/Admin/Documents/IT/Pociag-do-predykcji`; run commands directly,
not via `wsl bash -c "..."` wrappers.
