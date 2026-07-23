"""DAG: ingest_operations_daily — Ingest and process yesterday's operations."""

from __future__ import annotations

import logging
from datetime import date, datetime, timedelta
from typing import TypedDict

import httpx
from airflow.decorators import dag, task
from airflow.hooks.base import BaseHook

logger = logging.getLogger(__name__)


class FetchResult(TypedDict):
    target_date: str


class ProcessingResult(TypedDict):
    target_date: str
    status: str
    records_read: int
    records_written: int


class DailyIngestionResult(TypedDict):
    target_date: str
    operations: ProcessingResult
    disruptions: ProcessingResult


def target_date_from_data_interval(data_interval_end: datetime) -> date:
    return (data_interval_end - timedelta(days=1)).date()


@dag(
    dag_id="ingest_operations_daily",
    schedule="0 2 * * *",
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args={"retries": 2, "retry_delay": timedelta(minutes=5)},
    tags=["pociag", "ingestion"],
)
def ingest_operations_daily() -> None:
    @task
    def fetch_operations(data_interval_end: datetime) -> FetchResult:
        target_date = target_date_from_data_interval(data_interval_end)
        conn = BaseHook.get_connection("pociag_collector")
        base_url = f"{conn.schema}://{conn.host}:{conn.port}"
        response = httpx.post(
            f"{base_url}/api/v1/fetch/operations",
            json={"date": target_date.isoformat(), "force": False},
            timeout=600,
        )
        response.raise_for_status()
        return {"target_date": target_date.isoformat()}

    @task
    def process_operations(fetch_result: FetchResult) -> ProcessingResult:
        from pociag_processing.pipelines.operations import process_operations as run

        target_date = date.fromisoformat(fetch_result["target_date"])
        result = run(operating_date=target_date)
        return {
            "target_date": target_date.isoformat(),
            "status": result.status,
            "records_read": result.records_read,
            "records_written": result.records_written,
        }

    @task
    def fetch_disruptions(processing_result: ProcessingResult) -> ProcessingResult:
        target_date = processing_result["target_date"]
        conn = BaseHook.get_connection("pociag_collector")
        base_url = f"{conn.schema}://{conn.host}:{conn.port}"
        response = httpx.post(
            f"{base_url}/api/v1/fetch/disruptions",
            json={"date_from": target_date, "date_to": target_date, "force": False},
            timeout=600,
        )
        response.raise_for_status()
        return processing_result

    @task
    def process_disruptions(processing_result: ProcessingResult) -> DailyIngestionResult:
        from pociag_processing.pipelines.disruptions import process_disruptions as run

        target_date = date.fromisoformat(processing_result["target_date"])
        result = run(date_from=target_date, date_to=target_date)
        disruptions_result: ProcessingResult = {
            "target_date": target_date.isoformat(),
            "status": result.status,
            "records_read": result.records_read,
            "records_written": result.records_written,
        }
        return {
            "target_date": target_date.isoformat(),
            "operations": processing_result,
            "disruptions": disruptions_result,
        }

    @task
    def log_statistics(result: DailyIngestionResult) -> None:
        logger.info(
            "Daily ingestion complete for %s: operations read=%d written=%d, "
            "disruptions read=%d written=%d",
            result["target_date"],
            result["operations"]["records_read"],
            result["operations"]["records_written"],
            result["disruptions"]["records_read"],
            result["disruptions"]["records_written"],
        )

    operations_fetch = fetch_operations()  # type: ignore[call-arg]
    operations_processing = process_operations(operations_fetch)  # type: ignore[arg-type]
    disruptions_fetch = fetch_disruptions(operations_processing)  # type: ignore[arg-type]
    disruptions_processing = process_disruptions(disruptions_fetch)  # type: ignore[arg-type]
    log_statistics(disruptions_processing)  # type: ignore[arg-type]


ingest_operations_daily()
