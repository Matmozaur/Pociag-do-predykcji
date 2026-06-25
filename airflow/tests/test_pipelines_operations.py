from __future__ import annotations

from datetime import date
from unittest.mock import MagicMock, patch

import pytest

from pociag_processing.models import UpsertResult
from pociag_processing.pipelines.operations import process_operations


@patch("pociag_processing.pipelines.operations.LakeReader")
@patch("pociag_processing.pipelines.operations.SyncRepository")
def test_process_operations_success(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 1
    mock_lake.read_raw_operations.return_value = [
        {
            "metadata": {},
            "payload": {
                "operations": [
                    {
                        "scheduleId": 10,
                        "orderId": 1,
                        "trainOrderId": 100,
                        "trainStatus": "ON_TIME",
                        "stations": [
                            {
                                "stationId": 5,
                                "plannedSequenceNumber": 1,
                                "actualSequenceNumber": 1,
                                "plannedArrival": None,
                                "plannedDeparture": "2025-06-01T08:00:00+02:00",
                                "arrivalDelayMinutes": None,
                                "departureDelayMinutes": 0,
                                "actualArrival": None,
                                "actualDeparture": "2025-06-01T08:00:00+02:00",
                                "isConfirmed": True,
                                "isCancelled": False,
                            }
                        ],
                    }
                ]
            },
        }
    ]
    mock_repo.upsert_operations.return_value = UpsertResult(records_read=1, records_written=1)

    result = process_operations(operating_date=date(2025, 6, 1))

    assert result.status == "success"
    assert result.records_written == 1
    mock_repo.upsert_operations.assert_called_once()
    call_args = mock_repo.upsert_operations.call_args
    assert call_args[0][1] == date(2025, 6, 1)
    mock_repo.mark_processing_run_success.assert_called_once_with(1, 1, 1)


@patch("pociag_processing.pipelines.operations.LakeReader")
@patch("pociag_processing.pipelines.operations.SyncRepository")
def test_process_operations_pipeline_running(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_repo.is_pipeline_running.return_value = True

    result = process_operations(operating_date=date(2025, 6, 1))

    assert result.status == "failed"
    mock_repo.create_processing_run.assert_not_called()


@patch("pociag_processing.pipelines.operations.LakeReader")
@patch("pociag_processing.pipelines.operations.SyncRepository")
def test_process_operations_exception_marks_failed(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 5
    mock_lake.read_raw_operations.side_effect = RuntimeError("connection lost")

    with pytest.raises(RuntimeError, match="connection lost"):
        process_operations(operating_date=date(2025, 6, 1))

    mock_repo.mark_processing_run_failed.assert_called_once_with(5, "connection lost")


@patch("pociag_processing.pipelines.operations.LakeReader")
@patch("pociag_processing.pipelines.operations.SyncRepository")
def test_process_operations_empty_payload(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 1
    mock_lake.read_raw_operations.return_value = [
        {"metadata": {}, "payload": {"operations": []}}
    ]

    result = process_operations(operating_date=date(2025, 6, 1))

    assert result.status == "success"
    assert result.records_written == 0
    mock_repo.upsert_operations.assert_not_called()
