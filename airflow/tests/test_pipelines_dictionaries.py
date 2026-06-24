from __future__ import annotations

from datetime import date
from unittest.mock import MagicMock, patch

from pociag_processing.models import UpsertResult
from pociag_processing.pipelines.dictionaries import process_dictionaries


@patch("pociag_processing.pipelines.dictionaries.LakeReader")
@patch("pociag_processing.pipelines.dictionaries.SyncRepository")
def test_process_dictionaries_success(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 1
    mock_lake.read_raw_dictionaries.return_value = [
        {
            "metadata": {"dictionary_type": "carriers"},
            "payload": {"carriers": [{"code": "IC", "name": "InterCity"}]},
        }
    ]
    mock_repo.upsert_carriers.return_value = UpsertResult(records_read=1, records_written=1)

    result = process_dictionaries(run_date=date(2025, 6, 1))

    assert result.status == "success"
    assert result.records_read == 1
    assert result.records_written == 1
    mock_repo.mark_processing_run_success.assert_called_once_with(1, 1, 1)


@patch("pociag_processing.pipelines.dictionaries.LakeReader")
@patch("pociag_processing.pipelines.dictionaries.SyncRepository")
def test_process_dictionaries_pipeline_running(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_repo.is_pipeline_running.return_value = True

    result = process_dictionaries(run_date=date(2025, 6, 1))

    assert result.status == "failed"
    assert result.records_read == 0
    mock_repo.create_processing_run.assert_not_called()


@patch("pociag_processing.pipelines.dictionaries.LakeReader")
@patch("pociag_processing.pipelines.dictionaries.SyncRepository")
def test_process_dictionaries_exception_marks_failed(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 42
    mock_lake.read_raw_dictionaries.side_effect = RuntimeError("S3 error")

    import pytest

    with pytest.raises(RuntimeError, match="S3 error"):
        process_dictionaries(run_date=date(2025, 6, 1))

    mock_repo.mark_processing_run_failed.assert_called_once_with(42, "S3 error")


@patch("pociag_processing.pipelines.dictionaries.LakeReader")
@patch("pociag_processing.pipelines.dictionaries.SyncRepository")
def test_process_dictionaries_unknown_type_skipped(mock_repo_cls: MagicMock, mock_lake_cls: MagicMock) -> None:
    mock_repo = mock_repo_cls.return_value
    mock_lake = mock_lake_cls.return_value
    mock_repo.is_pipeline_running.return_value = False
    mock_repo.create_processing_run.return_value = 1
    mock_lake.read_raw_dictionaries.return_value = [
        {
            "metadata": {"dictionary_type": "unknown_thing"},
            "payload": {"unknown_thing": [{"id": 1}]},
        }
    ]

    result = process_dictionaries(run_date=date(2025, 6, 1))

    assert result.status == "success"
    assert result.records_written == 0
