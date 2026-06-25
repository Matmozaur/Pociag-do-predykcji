from __future__ import annotations

from datetime import date
from unittest.mock import MagicMock, patch

import pytest

from pociag_processing.models import UpsertResult
from pociag_processing.pipelines.disruptions import process_disruptions


@patch("pociag_processing.pipelines.disruptions.LakeReader")
@patch("pociag_processing.pipelines.disruptions.SyncRepository")
def test_process_disruptions_success(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 1
    mock_lake.read_raw_disruptions.return_value = [
        {
            "metadata": {},
            "payload": {
                "disruptions": [
                    {
                        "disruptionId": 999,
                        "disruptionTypeCode": "DELAY",
                        "startStationId": 10,
                        "endStationId": 20,
                        "message": "Track works",
                        "dateFrom": "2025-06-01",
                        "dateTo": "2025-06-07",
                        "affectedRoutes": [
                            {
                                "scheduleId": 1,
                                "orderId": 1,
                                "trainOrderId": 100,
                                "operatingDate": "2025-06-02",
                                "stationId": 10,
                                "sequenceNumber": 3,
                            }
                        ],
                    }
                ]
            },
        }
    ]
    mock_repo.upsert_disruptions.return_value = UpsertResult(records_read=1, records_written=1)

    result = process_disruptions(date_from=date(2025, 6, 1), date_to=date(2025, 6, 7))

    assert result.status == "success"
    assert result.records_written == 1
    mock_repo.upsert_disruptions.assert_called_once()
    mock_repo.mark_processing_run_success.assert_called_once_with(1, 1, 1)


@patch("pociag_processing.pipelines.disruptions.LakeReader")
@patch("pociag_processing.pipelines.disruptions.SyncRepository")
def test_process_disruptions_pipeline_running(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_repo.is_pipeline_running.return_value = True

    result = process_disruptions(date_from=date(2025, 6, 1), date_to=date(2025, 6, 7))

    assert result.status == "failed"
    mock_repo.create_processing_run.assert_not_called()


@patch("pociag_processing.pipelines.disruptions.LakeReader")
@patch("pociag_processing.pipelines.disruptions.SyncRepository")
def test_process_disruptions_exception_marks_failed(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 7
    mock_lake.read_raw_disruptions.side_effect = RuntimeError("network timeout")

    with pytest.raises(RuntimeError, match="network timeout"):
        process_disruptions(date_from=date(2025, 6, 1), date_to=date(2025, 6, 7))

    mock_repo.mark_processing_run_failed.assert_called_once_with(7, "network timeout")


@patch("pociag_processing.pipelines.disruptions.LakeReader")
@patch("pociag_processing.pipelines.disruptions.SyncRepository")
def test_process_disruptions_empty_payload(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 1
    mock_lake.read_raw_disruptions.return_value = [
        {"metadata": {}, "payload": {"disruptions": []}}
    ]

    result = process_disruptions(date_from=date(2025, 6, 1), date_to=date(2025, 6, 7))

    assert result.status == "success"
    assert result.records_written == 0
    mock_repo.upsert_disruptions.assert_not_called()
