"""DAG sync_disruptions — every 15 minutes: replace the disruption snapshot."""

from __future__ import annotations

from datetime import timedelta

import pendulum
from airflow.decorators import dag, task

WARSAW = pendulum.timezone("Europe/Warsaw")


@dag(
    dag_id="sync_disruptions",
    schedule="*/15 * * * *",
    start_date=pendulum.datetime(2026, 1, 1, tz=WARSAW),
    catchup=False,
    max_active_runs=1,
    dagrun_timeout=timedelta(minutes=14),
    default_args={"retries": 1, "retry_delay": timedelta(minutes=2)},
    tags=["pociag"],
)
def sync_disruptions() -> None:
    @task
    def disruptions() -> dict[str, int]:
        from pociag_processing.pipelines import sync_disruptions as run

        return run(pendulum.now(WARSAW).date())

    disruptions()


sync_disruptions()
