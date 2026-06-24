from __future__ import annotations

from datetime import date
from unittest.mock import MagicMock, patch

from pociag_processing.models import UpsertResult
from pociag_processing.pipelines.schedules import process_schedules


@patch("pociag_processing.pipelines.schedules.LakeReader")
@patch("pociag_processing.pipelines.schedules.SyncRepository")
def test_process_schedules_success(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 1
    mock_lake.read_raw_schedules.return_value = [
        {
            "metadata": {},
            "payload": {
                "routes": [
                    {"scheduleId": 1, "orderId": 1, "name": "Train A", "stations": []},
                ]
            },
        }
    ]
    mock_repo.upsert_routes.return_value = UpsertResult(records_read=1, records_written=1)

    result = process_schedules(date_from=date(2025, 6, 1), date_to=date(2025, 6, 7))

    assert result.status == "success"
    assert result.records_written == 1
    mock_repo.mark_processing_run_success.assert_called_once_with(1, 1, 1)


@patch("pociag_processing.pipelines.schedules.LakeReader")
@patch("pociag_processing.pipelines.schedules.SyncRepository")
def test_process_schedules_pipeline_running(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_repo.is_pipeline_running.return_value = True

    result = process_schedules(date_from=date(2025, 6, 1), date_to=date(2025, 6, 7))

    assert result.status == "failed"
    mock_repo.create_processing_run.assert_not_called()


@patch("pociag_processing.pipelines.schedules.LakeReader")
@patch("pociag_processing.pipelines.schedules.SyncRepository")
def test_process_schedules_empty_envelopes(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 1
    mock_lake.read_raw_schedules.return_value = [
        {"metadata": {}, "payload": {"routes": []}}
    ]

    result = process_schedules(date_from=date(2025, 6, 1), date_to=date(2025, 6, 7))

    assert result.status == "success"
    assert result.records_written == 0
    mock_repo.upsert_routes.assert_not_called()
