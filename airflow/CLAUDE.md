# airflow — DAGs + pociag_processing plugin

Stage-2 of the ELT: reads raw Parquet from MinIO, transforms it, upserts curated PostgreSQL
tables. This is an **Airflow plugin, not a standalone service**
(`docs/decisions/003-processor-to-airflow-plugin.md`). See repo root `CLAUDE.md` for architecture.

## Layout

- `dags/` — one file per pipeline (`ingest_operations_daily.py`, `ingest_schedules_weekly.py`,
  `sync_dictionaries_weekly.py`). TaskFlow API: `@dag` / `@task`. DAGs are thin orchestration.
- `plugins/pociag_processing/` — all real logic, a **separate installable package**
  (`plugins/pyproject.toml`, name `pociag-processing`):
  - `pipelines/{dictionaries,schedules,operations,disruptions}.py` — `process_*()` entrypoints
    called from DAG tasks.
  - `repository.py` — `SyncRepository`, all SQL. `lake.py` — `LakeReader` (Parquet from MinIO).
  - `models.py` — `ProcessResult` / `UpsertResult` / `PipelineName`. `tracing.py` — `get_tracer()`.

## Setup & commands (run from `airflow/`)

```bash
cd airflow
uv sync --all-extras && uv pip install -e plugins   # first time; plugin must be editable-installed
uv run pytest tests/ -v                             # single: uv run pytest tests/test_pipelines_operations.py::test_name -v
uv run ruff check .                                 # line-length 100, target py312
uv run mypy plugins/pociag_processing               # strict; must be clean
```

CI does exactly this (`uv sync --all-extras`, `uv pip install -e plugins`, then ruff/mypy/pytest).

## Testing DAGs locally

- `tests/` are **plain unit tests** over `process_*` pipeline functions and `SyncRepository`,
  using `unittest.mock` / `pytest-mock`. They `@patch("pociag_processing.repository.PostgresHook")`
  and assert on cursor `execute` / `commit` calls. **No `conftest.py`, no `DagBag` test, no local
  Airflow DB or scheduler needed.**
- To check a DAG actually parses/schedules, start Airflow (`make infra-up-airflow`, UI
  http://localhost:8090) and look there.

## Conventions

- Full type annotations; `mypy --strict` clean. `from __future__ import annotations` at top of
  every module. TaskFlow call sites carry `# type: ignore[...]` for the decorator's dynamic
  signature — that's expected, keep it minimal.
- External systems **only via Airflow connections/hooks**: `pociag_postgres` (`PostgresHook` in
  `repository.py`), `pociag_s3` (`LakeReader`, default `conn_id`), `pociag_collector` (DAGs call
  `BaseHook.get_connection` then `httpx.post`). Never instantiate clients with inline credentials.
- **Idempotent & re-runnable** for any execution date: pipelines guard with
  `repository.is_pipeline_running(...)` and record runs via `create_processing_run` /
  `mark_processing_run_success`. Upserts, not inserts.
- Parameterized SQL only (`cursor.execute(sql, params)`); keep all SQL in `repository.py`.
- Event/record shapes must match `specs/asyncapi/` and `specs/schemas/`; table shapes must match
  `db/migrations/`. `plugins/pociag_processing/data/*.json` is packaged (station coordinates etc.).
