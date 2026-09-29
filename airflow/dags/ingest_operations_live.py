"""DAG: ingest_operations_live — Refresh current operations every 10 minutes."""

from __future__ import annotations

import logging
from datetime import date, datetime, timedelta
from typing import Any, TypedDict

import httpx
from airflow.decorators import dag, task
from airflow.hooks.base import BaseHook

logger = logging.getLogger(__name__)


class FetchResult(TypedDict):
    target_date: str
    ingestion_run_id: int | None
    skipped: bool


class ProcessingResult(TypedDict):
    target_date: str
    status: str
    records_read: int
    records_written: int


def _fetch_run_id(fetch_result: object) -> int | None:
    if not isinstance(fetch_result, dict):
        return None
    value: Any = fetch_result.get("run_id", fetch_result.get("runId"))
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    if isinstance(value, str) and value.isdecimal():
        return int(value)
    return None


def capture_date_today() -> date:
    return date.today()


def fetch_operations_snapshot() -> FetchResult:
    capture_date = capture_date_today()
    conn = BaseHook.get_connection("pociag_collector")
    base_url = f"{conn.schema}://{conn.host}:{conn.port}"
    response = httpx.post(
        f"{base_url}/api/v1/fetch/operations",
        json={"force": False},
        timeout=480,
    )
    if response.status_code == httpx.codes.CONFLICT:
        logger.info("Operations fetch already running; skipping this live run")
        return {
            "target_date": capture_date.isoformat(),
            "ingestion_run_id": None,
            "skipped": True,
        }
    response.raise_for_status()
    return {
        "target_date": capture_date.isoformat(),
        "ingestion_run_id": _fetch_run_id(response.json()),
        "skipped": False,
    }


def process_operations_snapshot(fetch_result: FetchResult) -> ProcessingResult:
    capture_date = date.fromisoformat(fetch_result["target_date"])
    if fetch_result["skipped"]:
        return {
            "target_date": capture_date.isoformat(),
            "status": "skipped",
            "records_read": 0,
            "records_written": 0,
        }

    from pociag_processing.pipelines.operations import process_operations as run

    result = run(
        capture_date=capture_date,
        ingestion_run_id=fetch_result["ingestion_run_id"],
    )
    return {
        "target_date": capture_date.isoformat(),
        "status": result.status,
        "records_read": result.records_read,
        "records_written": result.records_written,
    }


@dag(
    dag_id="ingest_operations_live",
    schedule="*/10 * * * *",
    start_date=datetime(2025, 1, 1),
    catchup=False,
    max_active_runs=1,
    dagrun_timeout=timedelta(minutes=9),
    default_args={"retries": 1, "retry_delay": timedelta(minutes=1)},
    tags=["pociag", "ingestion"],
)
def ingest_operations_live() -> None:
    @task
    def fetch_operations() -> FetchResult:
        return fetch_operations_snapshot()

    @task
    def process_operations(fetch_result: FetchResult) -> ProcessingResult:
        return process_operations_snapshot(fetch_result)

    process_operations(fetch_operations())  # type: ignore[arg-type]


ingest_operations_live()
