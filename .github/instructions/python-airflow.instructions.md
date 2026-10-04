---
description: "Python conventions for Airflow DAGs and the pociag_processing plugin."
applyTo: "airflow/**/*.py"
---

<!-- Mirror of airflow/CLAUDE.md -->

# airflow — DAGs + pociag_processing plugin

The only ingestion path: PLK Open Data API → PostgreSQL. Spec: `specs/pipelines.md`.

## Layout

- `dags/` — `sync_schedules.py`, `sync_operations.py`, `sync_disruptions.py`. Thin TaskFlow
  DAGs; each task imports and calls one `pociag_processing.pipelines.sync_*` function.
- `plugins/pociag_processing/` (installable package `pociag-processing`):
  - `plk.py` — `PlkClient` (httpx, `pociag_plk` connection).
  - `transform.py` — pure PLK payload → row dataclasses (`models.py`). All PLK quirks live here.
  - `repository.py` — `Repository`, all SQL, one transaction per batch (`pociag_postgres`).
  - `pipelines.py` — `sync_stations/schedules/operations/disruptions`; client and repository
    are injectable for tests.
  - `data/station_coordinates.json` — curated coordinates (regenerate with
    `scripts/build_station_coordinates.py`).
- `tests/` — `test_transform.py` (real trimmed PLK payloads in `fixtures/`), `test_pipelines.py`
  (fakes), `test_dags.py` (DagBag), `test_repository_integration.py` (needs
  `POCIAG_TEST_DATABASE_URL`; recreates that DB's `public` schema).

## Commands (from `airflow/`)

```bash
uv sync --all-extras && uv pip install -e plugins
uv run pytest tests -q
uv run ruff check . && uv run mypy plugins/pociag_processing dags   # strict
```

## Conventions

- `from __future__ import annotations`, full annotations, `mypy --strict` clean.
- External systems only via Airflow connections; SQL only in `repository.py`, parameterised.
- Syncs are idempotent upserts (disruptions: snapshot replace). Keep table shapes aligned with
  `db/migrations/`.
- The scheduler imports `plugins/` at start: **restart `airflow-scheduler` after plugin changes**.
