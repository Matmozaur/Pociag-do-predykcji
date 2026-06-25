"""DAG: process_disruptions_daily — Process train disruptions data."""

from __future__ import annotations

from datetime import date, datetime, timedelta

from airflow.decorators import dag, task


@dag(
    dag_id="process_disruptions_daily",
    schedule="0 7 * * *",
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args={"retries": 2, "retry_delay": timedelta(minutes=5)},
    tags=["pociag", "processing"],
)
def process_disruptions_daily() -> None:
    @task
    def process_disruptions() -> dict[str, str | int]:
        from pociag_processing.pipelines.disruptions import (
            process_disruptions as run,
        )

        yesterday = date.today() - timedelta(days=1)
        today = date.today()
        result = run(date_from=yesterday, date_to=today)
        return {
            "status": result.status,
            "records_read": result.records_read,
            "records_written": result.records_written,
        }

    process_disruptions()


process_disruptions_daily()
