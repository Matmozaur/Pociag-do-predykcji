"""DAG: sync_dictionaries_weekly — Sync reference data (stations, carriers, categories, stop types)."""

from __future__ import annotations

from datetime import date, datetime, timedelta

import httpx
from airflow.decorators import dag, task
from airflow.hooks.base import BaseHook


@dag(
    dag_id="sync_dictionaries_weekly",
    schedule="0 3 * * 1",
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args={"retries": 2, "retry_delay": timedelta(minutes=5)},
    tags=["pociag", "ingestion"],
)
def sync_dictionaries_weekly() -> None:
    @task
    def fetch_dictionaries() -> dict[str, object]:
        conn = BaseHook.get_connection("pociag_collector")
        base_url = f"{conn.schema}://{conn.host}:{conn.port}"
        response = httpx.post(
            f"{base_url}/api/v1/fetch/dictionaries",
            json={"force": False},
            timeout=300,
        )
        response.raise_for_status()
        return response.json()

    @task
    def process_dictionaries(fetch_result: dict[str, object]) -> dict[str, str | int]:
        from pociag_processing.pipelines.dictionaries import (
            process_dictionaries as run,
        )

        result = run(run_date=date.today(), ingestion_run_id=fetch_result.get("run_id"))
        return {
            "status": result.status,
            "records_written": result.records_written,
            "records_read": result.records_read,
        }

    result = fetch_dictionaries()
    process_dictionaries(result)


sync_dictionaries_weekly()
