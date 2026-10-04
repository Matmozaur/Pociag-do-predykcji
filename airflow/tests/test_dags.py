from __future__ import annotations

from pathlib import Path

import pytest
from airflow.models import DagBag

DAGS = Path(__file__).resolve().parents[1] / "dags"


@pytest.fixture(scope="module")
def dagbag() -> DagBag:
    return DagBag(dag_folder=str(DAGS), include_examples=False)


def test_dags_import_cleanly(dagbag: DagBag) -> None:
    assert dagbag.import_errors == {}
    assert set(dagbag.dag_ids) == {"sync_schedules", "sync_operations", "sync_disruptions"}


@pytest.mark.parametrize(
    ("dag_id", "schedule", "tasks"),
    [
        ("sync_schedules", "30 2 * * *", {"stations", "schedules"}),
        ("sync_operations", "*/10 * * * *", {"operations"}),
        ("sync_disruptions", "*/15 * * * *", {"disruptions"}),
    ],
)
def test_dag_shape(dagbag: DagBag, dag_id: str, schedule: str, tasks: set[str]) -> None:
    dag = dagbag.dags[dag_id]
    assert dag.schedule_interval == schedule
    assert dag.catchup is False
    assert dag.max_active_runs == 1
    assert dag.timezone.name == "Europe/Warsaw"
    assert set(dag.task_ids) == tasks


def test_schedules_runs_after_stations(dagbag: DagBag) -> None:
    dag = dagbag.dags["sync_schedules"]
    assert dag.get_task("schedules").upstream_task_ids == {"stations"}
