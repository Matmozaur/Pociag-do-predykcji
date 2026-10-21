---
mode: agent
description: Create a new Airflow DAG using TaskFlow API following project conventions
---

> Document the DAG's schedule, PLK requests and written tables in `specs/pipelines.md`.

Create a new Airflow DAG named **`${input:dagId}`** in `airflow/dags/${input:dagId}.py`.

## Requirements

The DAG should implement: **${input:dagDescription}**

## Conventions to follow

- Use `@dag` + `@task` decorators (TaskFlow API) throughout. Follow the existing DAGs in
  `airflow/dags/` (see `airflow/CLAUDE.md`).
- Set `schedule="${input:schedule}"` (cron, Europe/Warsaw), `catchup=False`,
  `max_active_runs=1`, `start_date` a `pendulum` datetime in Europe/Warsaw, `tags=["pociag"]`.
- Set `default_args` retries to fit the cadence (e.g. `{"retries": 2, "retry_delay": timedelta(minutes=10)}`
  daily, `{"retries": 0}` when the next run is minutes away) and a `dagrun_timeout` shorter than the interval.
- **The DAG stays thin**: each `@task` imports (inside the task, to keep parsing cheap) and calls
  one `pociag_processing.pipelines.sync_*` function and returns its counts dict.
- The logic goes in `airflow/plugins/pociag_processing/`: PLK HTTP via `PlkClient` (`plk.py`,
  `pociag_plk` connection), payload → rows in `transform.py`, SQL only in `repository.py`
  (`pociag_postgres` connection), wiring in `pipelines.py` with injectable client/repository.
- Tasks must be **idempotent** — upserts or snapshot replace, safe to re-run.
- Log with `logging.getLogger(__name__)`; trace with `pociag_processing.tracing.get_tracer()`.
- Add a module docstring stating the DAG's cadence and purpose.
- Add the DAG to `tests/test_dags.py` and the pipeline to `tests/test_pipelines.py`.

## Structure

```python
# airflow/dags/${input:dagId}.py
"""DAG ${input:dagId} — <cadence>: <what it syncs>."""

from __future__ import annotations

from datetime import timedelta

import pendulum
from airflow.decorators import dag, task

WARSAW = pendulum.timezone("Europe/Warsaw")


@dag(
    dag_id="${input:dagId}",
    schedule="${input:schedule}",
    start_date=pendulum.datetime(2026, 1, 1, tz=WARSAW),
    catchup=False,
    max_active_runs=1,
    default_args={"retries": 1, "retry_delay": timedelta(minutes=2)},
    tags=["pociag"],
)
def ${input:dagId}() -> None:
    @task
    def sync() -> dict[str, int]:
        from pociag_processing.pipelines import sync_<name> as run

        return run()

    sync()


${input:dagId}()
```

Generate the DAG file and the plugin code it calls, with tests.
