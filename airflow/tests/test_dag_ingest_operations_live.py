from __future__ import annotations

import importlib.util
from datetime import date, timedelta
from pathlib import Path
from types import ModuleType
from unittest.mock import MagicMock, patch

import httpx
import pytest

from pociag_processing.models import ProcessResult

DAG_PATH = Path(__file__).resolve().parents[1] / "dags" / "ingest_operations_live.py"


@pytest.fixture(scope="module")
def dag_module() -> ModuleType:
    spec = importlib.util.spec_from_file_location("ingest_operations_live", DAG_PATH)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _conn() -> MagicMock:
    conn = MagicMock()
    conn.schema = "http"
    conn.host = "collector"
    conn.port = 8081
    return conn


def test_dag_definition(dag_module: ModuleType) -> None:
    dag = dag_module.ingest_operations_live()

    assert dag.dag_id == "ingest_operations_live"
    assert dag.schedule_interval == "*/10 * * * *"
    assert dag.max_active_runs == 1
    assert dag.catchup is False
    assert dag.dagrun_timeout == timedelta(minutes=9)
    assert dag.default_args["retries"] == 0
    assert set(dag.tags) == {"pociag", "ingestion"}
    assert [t.task_id for t in dag.topological_sort()] == ["fetch_operations", "process_operations"]


def test_fetch_operations_returns_run_id(dag_module: ModuleType) -> None:
    response = httpx.Response(
        200,
        json={"run_id": 42, "lake_prefix": "raw/operations/2026/09/27/"},
        request=httpx.Request("POST", "http://collector:8081/api/v1/fetch/operations"),
    )
    with (
        patch.object(dag_module.BaseHook, "get_connection", return_value=_conn()),
        patch.object(dag_module.httpx, "post", return_value=response) as mock_post,
        patch.object(dag_module, "capture_date_today", return_value=date(2026, 9, 28)),
    ):
        result = dag_module.fetch_operations_snapshot()

    mock_post.assert_called_once()
    assert mock_post.call_args.args[0] == "http://collector:8081/api/v1/fetch/operations"
    assert mock_post.call_args.kwargs["json"] == {"force": False}
    assert result == {"target_date": "2026-09-27", "ingestion_run_id": 42, "skipped": False}


@pytest.mark.parametrize(
    ("body", "expected"),
    [
        ({"lake_prefix": "raw/operations/2026/09/27/"}, date(2026, 9, 27)),
        ({"lake_prefix": "raw/operations/2026/01/05/run_7/"}, date(2026, 1, 5)),
    ],
)
def test_capture_date_from_lake_prefix(
    dag_module: ModuleType, body: dict[str, str], expected: date
) -> None:
    assert dag_module.capture_date_from_lake_prefix(body) == expected


@pytest.mark.parametrize(
    "body",
    [
        {},
        {"lake_prefix": ""},
        {"lake_prefix": "raw/disruptions/2026/09/27/"},
        {"lake_prefix": "raw/operations/2026/13/27/"},
        None,
    ],
)
def test_capture_date_from_lake_prefix_rejects_invalid(
    dag_module: ModuleType, body: object
) -> None:
    with pytest.raises(ValueError):
        dag_module.capture_date_from_lake_prefix(body)


def test_fetch_operations_conflict_is_skipped(dag_module: ModuleType) -> None:
    response = httpx.Response(
        409,
        json={"error": "already_running"},
        request=httpx.Request("POST", "http://collector:8081/api/v1/fetch/operations"),
    )
    with (
        patch.object(dag_module.BaseHook, "get_connection", return_value=_conn()),
        patch.object(dag_module.httpx, "post", return_value=response),
        patch.object(dag_module, "capture_date_today", return_value=date(2026, 9, 27)),
    ):
        result = dag_module.fetch_operations_snapshot()

    assert result == {"target_date": "2026-09-27", "ingestion_run_id": None, "skipped": True}


def test_fetch_operations_other_errors_raise(dag_module: ModuleType) -> None:
    response = httpx.Response(
        500,
        request=httpx.Request("POST", "http://collector:8081/api/v1/fetch/operations"),
    )
    with (
        patch.object(dag_module.BaseHook, "get_connection", return_value=_conn()),
        patch.object(dag_module.httpx, "post", return_value=response),
        pytest.raises(httpx.HTTPStatusError),
    ):
        dag_module.fetch_operations_snapshot()


@patch("pociag_processing.pipelines.operations.process_operations")
def test_process_operations_passes_run_id(mock_run: MagicMock, dag_module: ModuleType) -> None:
    mock_run.return_value = ProcessResult(
        pipeline="operations",
        status="success",
        records_read=10,
        records_written=9,
        records_skipped=0,
        duration_ms=1,
    )

    result = dag_module.process_operations_snapshot(
        {"target_date": "2026-09-27", "ingestion_run_id": 42, "skipped": False}
    )

    mock_run.assert_called_once_with(capture_date=date(2026, 9, 27), ingestion_run_id=42)
    assert result["status"] == "success"
    assert result["records_written"] == 9


@patch("pociag_processing.pipelines.operations.process_operations")
def test_process_operations_skipped_does_nothing(
    mock_run: MagicMock, dag_module: ModuleType
) -> None:
    result = dag_module.process_operations_snapshot(
        {"target_date": "2026-09-27", "ingestion_run_id": None, "skipped": True}
    )

    mock_run.assert_not_called()
    assert result["status"] == "skipped"
