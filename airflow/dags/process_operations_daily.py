"""DAG: process_operations_daily — Process train operations data."""

from __future__ import annotations

from datetime import date, datetime, timedelta

from airflow.decorators import dag, task


@dag(
    dag_id="process_operations_daily",
    schedule="0 6 * * *",
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args={"retries": 2, "retry_delay": timedelta(minutes=5)},
    tags=["pociag", "processing"],
)
def process_operations_daily() -> None:
    @task
    def process_operations() -> dict[str, str | int]:
        from pociag_processing.pipelines.operations import (
            process_operations as run,
        )

        yesterday = date.today() - timedelta(days=1)
        result = run(operating_date=yesterday)
        return {
            "status": result.status,
            "records_read": result.records_read,
            "records_written": result.records_written,
        }

    process_operations()


process_operations_daily()
