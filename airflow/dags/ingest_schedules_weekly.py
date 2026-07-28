"""DAG: ingest_schedules_weekly — Pull train schedules for the next 2 weeks."""

from __future__ import annotations

from datetime import date, datetime, timedelta
from typing import Any, cast

import httpx
from airflow.decorators import dag, task
from airflow.hooks.base import BaseHook
from airflow.sensors.external_task import ExternalTaskSensor


def _fetch_run_id(fetch_result: object) -> int | None:
    if not isinstance(fetch_result, dict):
        return None
    value: Any = fetch_result.get("run_id", fetch_result.get("runId"))
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    if isinstance(value, str) and value.isdecimal():
        return int(value)
    return None


@dag(
    dag_id="ingest_schedules_weekly",
    schedule="0 4 * * 1",
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args={"retries": 2, "retry_delay": timedelta(minutes=5)},
    tags=["pociag", "ingestion"],
)
def ingest_schedules_weekly() -> None:
    @task
    def check_last_run() -> bool:
        from pociag_processing.repository import SyncRepository

        repo = SyncRepository()
        runs = repo.list_processing_runs("schedules", 1)
        if not runs:
            return True
        last_run = runs[0]
        completed_at = last_run.get("completed_at")
        if completed_at is None:
            return True
        if isinstance(completed_at, str):
            completed_at = datetime.fromisoformat(completed_at)
        return (datetime.now(tz=completed_at.tzinfo) - completed_at) > timedelta(days=7)

    @task.short_circuit
    def should_proceed(needs_run: bool) -> bool:
        return needs_run

    @task
    def fetch_schedules() -> dict[str, object]:
        conn = BaseHook.get_connection("pociag_collector")
        base_url = f"{conn.schema}://{conn.host}:{conn.port}"
        today = datetime.now().strftime("%Y-%m-%d")
        date_to = (datetime.now() + timedelta(days=14)).strftime("%Y-%m-%d")
        response = httpx.post(
            f"{base_url}/api/v1/fetch/schedules",
            json={"date_from": today, "date_to": date_to, "force": False},
            timeout=600,
        )
        response.raise_for_status()
        return cast(dict[str, object], response.json())

    @task
    def process_schedules(fetch_result: dict[str, object]) -> dict[str, str | int]:
        from pociag_processing.pipelines.schedules import (
            process_schedules as run,
        )

        today = date.today()
        date_to = today + timedelta(days=14)
        result = run(
            date_from=today,
            date_to=date_to,
            ingestion_run_id=_fetch_run_id(fetch_result),
        )
        return {
            "status": result.status,
            "records_written": result.records_written,
            "records_read": result.records_read,
        }

    @task
    def verify_result(result: dict[str, str | int]) -> None:
        records = result.get("records_written", 0)
        status = result.get("status", "unknown")
        if status != "success":
            raise ValueError(f"Processing failed with status: {status}")
        print(f"Schedules processed successfully: {records} records written")

    needs_run = check_last_run()
    proceed = should_proceed(needs_run)  # type: ignore[arg-type]
    dictionaries_complete = ExternalTaskSensor(
        task_id="wait_for_dictionaries",
        external_dag_id="sync_dictionaries_weekly",
        external_task_id="process_dictionaries",
        allowed_states=["success"],
        failed_states=["failed", "upstream_failed"],
        mode="reschedule",
        poke_interval=60,
        timeout=3600,
    )
    fetch_result = fetch_schedules()
    process_result = process_schedules(fetch_result)  # type: ignore[arg-type]
    verify_result(process_result)  # type: ignore[arg-type]

    proceed >> dictionaries_complete >> fetch_result >> process_result


ingest_schedules_weekly()
