---
mode: agent
description: Create a new Airflow DAG using TaskFlow API following project conventions
---

> If this DAG consumes or produces events, check `specs/asyncapi/` for the relevant channel
> definitions and align task inputs/outputs with the message schemas there.

Create a new Airflow DAG named **`${input:dagId}`** in `airflow/dags/${input:dagId}.py`.

## Requirements

The DAG should implement: **${input:dagDescription}**

## Conventions to follow

- Use `@dag` + `@task` decorators (TaskFlow API) throughout.
- Set `schedule="${input:schedule}"` (default `@daily`) and `catchup=False`.
- Include `default_args = {"retries": 2, "retry_delay": timedelta(minutes=5)}`.
- All database connections via `PostgresHook(postgres_conn_id="postgres_main")`.
- All external HTTP calls via `HttpHook`.
- Tasks must be **idempotent** — safe to re-run for the same `logical_date`.
- Log with `logging.getLogger(__name__)` (standard Airflow logger).
- Add a docstring to the DAG function describing its business purpose.
- If a task is CPU/memory heavy, use `DockerOperator` pointing to the relevant service image.
- Add a `@task` named `notify_on_failure` and wire it via `dag.on_failure_callback`.

## Structure

```python
# airflow/dags/${input:dagId}.py
"""
<business description of the DAG>
"""
import logging
from datetime import datetime, timedelta
from airflow.decorators import dag, task

log = logging.getLogger(__name__)

@dag(
    dag_id="${input:dagId}",
    schedule=...,
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args={...},
    tags=["pociag"],
)
def ${input:dagId}():
    @task
    def extract(...) -> dict: ...

    @task
    def transform(data: dict) -> dict: ...

    @task
    def load(data: dict) -> None: ...

    load(transform(extract()))

${input:dagId}()
```

Generate the complete DAG file with all tasks implemented.
