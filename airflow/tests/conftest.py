from __future__ import annotations

from datetime import date
from typing import Any
from unittest.mock import MagicMock, patch

import pytest

from pociag_processing.models import UpsertResult


@pytest.fixture
def mock_repo() -> MagicMock:
    with patch("pociag_processing.repository.SyncRepository") as cls:
        repo = cls.return_value
        repo.is_pipeline_running.return_value = False
        repo.create_processing_run.return_value = 1
        repo.mark_processing_run_success.return_value = None
        repo.mark_processing_run_failed.return_value = None
        repo.upsert_carriers.return_value = UpsertResult(records_read=0, records_written=0)
        repo.upsert_stations.return_value = UpsertResult(records_read=0, records_written=0)
        repo.upsert_commercial_categories.return_value = UpsertResult(records_read=0, records_written=0)
        repo.upsert_stop_types.return_value = UpsertResult(records_read=0, records_written=0)
        repo.upsert_routes.return_value = UpsertResult(records_read=0, records_written=0)
        repo.upsert_operations.return_value = UpsertResult(records_read=0, records_written=0)
        repo.upsert_disruptions.return_value = UpsertResult(records_read=0, records_written=0)
        yield repo


@pytest.fixture
def mock_lake() -> MagicMock:
    with patch("pociag_processing.lake.LakeReader") as cls:
        lake = cls.return_value
        lake.read_raw_dictionaries.return_value = []
        lake.read_raw_schedules.return_value = []
        lake.read_raw_operations.return_value = []
        lake.read_raw_disruptions.return_value = []
        yield lake
