"""DAG sync_schedules — daily: stations, then trains and planned stops for the next days."""

from __future__ import annotations

from datetime import timedelta

import pendulum
from airflow.decorators import dag, task

WARSAW = pendulum.timezone("Europe/Warsaw")
DAYS_AHEAD = 7  # today plus the next 7 days


@dag(
    dag_id="sync_schedules",
    schedule="30 2 * * *",
    start_date=pendulum.datetime(2026, 1, 1, tz=WARSAW),
    catchup=False,
    max_active_runs=1,
    default_args={"retries": 2, "retry_delay": timedelta(minutes=10)},
    tags=["pociag"],
)
def sync_schedules() -> None:
    @task
    def stations() -> dict[str, int]:
        from pociag_processing.pipelines import sync_stations

        return sync_stations()

    @task
    def schedules() -> dict[str, int]:
        from pociag_processing.pipelines import sync_schedules as run

        today = pendulum.now(WARSAW).date()
        return run([today + timedelta(days=n) for n in range(DAYS_AHEAD + 1)])

    stations() >> schedules()


sync_schedules()
