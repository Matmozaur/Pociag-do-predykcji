"""DAG: ingest_schedules_weekly — Pull train schedules for the next 2 weeks."""

from __future__ import annotations

from datetime import date, datetime, timedelta

import httpx
from airflow.decorators import dag, task
from airflow.hooks.base import BaseHook


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
        return response.json()

    @task
    def process_schedules(fetch_result: dict[str, object]) -> dict[str, str | int]:
        from pociag_processing.pipelines.schedules import (
            process_schedules as run,
        )

        today = date.today()
        date_to = today + timedelta(days=14)
        result = run(date_from=today, date_to=date_to, ingestion_run_id=fetch_result.get("run_id"))
        return {
            "status": result.status,
            "records_written": result.records_written,
            "records_read": result.records_read,
        }

    @task
    def verify_result(result: dict) -> None:
        records = result.get("records_written", 0)
        status = result.get("status", "unknown")
        if status != "success":
            raise ValueError(f"Processing failed with status: {status}")
        print(f"Schedules processed successfully: {records} records written")

    needs_run = check_last_run()
    proceed = should_proceed(needs_run)
    fetch_result = fetch_schedules()
    process_result = process_schedules(fetch_result)
    verify_result(process_result)

    proceed >> fetch_result >> process_result


ingest_schedules_weekly()
