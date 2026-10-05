"""DAG sync_operations — every 10 minutes: the current PLK operations snapshot."""

from __future__ import annotations

from datetime import timedelta

import pendulum
from airflow.decorators import dag, task

WARSAW = pendulum.timezone("Europe/Warsaw")


@dag(
    dag_id="sync_operations",
    schedule="*/10 * * * *",
    start_date=pendulum.datetime(2026, 1, 1, tz=WARSAW),
    catchup=False,
    max_active_runs=1,
    dagrun_timeout=timedelta(minutes=9),
    default_args={"retries": 0},  # the next run is at most 10 minutes away
    tags=["pociag"],
)
def sync_operations() -> None:
    @task
    def operations() -> dict[str, int]:
        from pociag_processing.pipelines import sync_operations as run

        return run()

    operations()


sync_operations()
