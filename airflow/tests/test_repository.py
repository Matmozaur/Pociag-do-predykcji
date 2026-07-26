from __future__ import annotations

import logging
from datetime import date
from unittest.mock import MagicMock, patch

import pytest

from pociag_processing.repository import SyncRepository


@patch("pociag_processing.repository.PostgresHook")
def test_upsert_station_cities_updates_existing_stations(mock_hook_cls: MagicMock) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_cursor.rowcount = 1
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn

    result = SyncRepository().upsert_station_cities(
        [{"name": "WARSZAWA", "stationIds": [10, 11]}]
    )

    assert result.records_read == 2
    assert result.records_written == 2
    assert mock_cursor.execute.call_count == 2
    mock_conn.commit.assert_called_once()


@patch("pociag_processing.repository.PostgresHook")
def test_upsert_routes_persists_connections_and_operating_dates(mock_hook_cls: MagicMock) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn
    mock_cursor.fetchone.side_effect = [(42,), (99,)]

    result = SyncRepository().upsert_routes(
        [
            {
                "scheduleId": 10,
                "orderId": 1,
                "operatingDates": ["2025-06-01"],
                "stations": [],
                "connections": [
                    {
                        "id": "connection-1",
                        "typeCode": "POLAC",
                        "stationId": 5,
                        "train1OrderId": 1,
                        "train2OrderId": 2,
                        "operatingDates": ["2025-06-01", "2025-06-02"],
                    }
                ],
            }
        ]
    )

    assert result.records_written == 1
    assert mock_cursor.execute.call_count == 8
    executed_sql = [call.args[0] for call in mock_cursor.execute.call_args_list]
    assert any("DELETE FROM route_operating_dates" in query for query in executed_sql)
    assert any("DELETE FROM route_connections" in query for query in executed_sql)
    assert any("INSERT INTO route_connections" in query for query in executed_sql)
    assert sum("route_connection_operating_dates" in query for query in executed_sql) == 2
    assert any("DELETE FROM route_stations" in query for query in executed_sql)


@patch("pociag_processing.repository.PostgresHook")
def test_upsert_routes_skips_connections_without_nonempty_type_code(
    mock_hook_cls: MagicMock, caplog: pytest.LogCaptureFixture
) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn
    mock_cursor.fetchone.return_value = (42,)

    with caplog.at_level(logging.WARNING):
        SyncRepository().upsert_routes(
            [
                {
                    "scheduleId": 10,
                    "orderId": 1,
                    "connections": [{"id": "bad", "typeCode": None}],
                }
            ]
        )

    executed_sql = [call.args[0] for call in mock_cursor.execute.call_args_list]
    assert not any("INSERT INTO route_connections" in query for query in executed_sql)
    assert "Skipping route connection without a nonempty typeCode" in caplog.text


@patch("pociag_processing.repository.PostgresHook")
def test_upsert_station_cities_skips_records_without_nonempty_name(
    mock_hook_cls: MagicMock,
) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn

    result = SyncRepository().upsert_station_cities([{"name": " ", "stationIds": [10]}])

    assert result.records_read == 0
    assert result.records_written == 0
    mock_cursor.execute.assert_not_called()


@patch("pociag_processing.repository.PostgresHook")
def test_upsert_operations_executes_correct_queries(mock_hook_cls: MagicMock) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn
    mock_cursor.fetchone.return_value = (42,)

    repo = SyncRepository()
    operations = [
        {
            "scheduleId": 10,
            "orderId": 1,
            "trainOrderId": 100,
            "operatingDate": "2025-06-01",
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

    result = repo.upsert_operations(operations)

    assert result.records_written == 1
    assert result.records_read == 1
    assert mock_cursor.execute.call_count == 2  # 1 op + 1 station
    mock_conn.commit.assert_called_once()


@patch("pociag_processing.repository.PostgresHook")
def test_upsert_disruptions_executes_correct_queries(mock_hook_cls: MagicMock) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn
    mock_cursor.fetchone.return_value = (99,)

    repo = SyncRepository()
    disruptions = [
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

    result = repo.upsert_disruptions(disruptions)

    assert result.records_written == 1
    assert result.records_read == 1
    # disruption insert + delete routes + insert route = 3 calls
    assert mock_cursor.execute.call_count == 3
    mock_conn.commit.assert_called_once()


@patch("pociag_processing.repository.PostgresHook")
def test_upsert_operations_rollback_on_error(mock_hook_cls: MagicMock) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn
    mock_cursor.fetchone.return_value = None  # simulate failure

    repo = SyncRepository()
    operations = [
        {"scheduleId": 1, "orderId": 1, "trainOrderId": None, "operatingDate": "2025-06-01", "trainStatus": "DELAYED", "stations": []}
    ]

    with pytest.raises(RuntimeError, match="failed to upsert train operation"):
        repo.upsert_operations(operations)

    mock_conn.rollback.assert_called_once()


@patch("pociag_processing.repository.PostgresHook")
def test_is_pipeline_running_true(mock_hook_cls: MagicMock) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn
    mock_cursor.fetchone.return_value = (True,)

    repo = SyncRepository()
    assert repo.is_pipeline_running("operations", date(2025, 6, 1)) is True


@patch("pociag_processing.repository.PostgresHook")
def test_is_pipeline_running_false(mock_hook_cls: MagicMock) -> None:
    mock_conn = MagicMock()
    mock_cursor = MagicMock()
    mock_conn.cursor.return_value.__enter__ = MagicMock(return_value=mock_cursor)
    mock_conn.cursor.return_value.__exit__ = MagicMock(return_value=False)
    mock_hook_cls.return_value.get_conn.return_value = mock_conn
    mock_cursor.fetchone.return_value = (False,)

    repo = SyncRepository()
    assert repo.is_pipeline_running("operations", date(2025, 6, 1)) is False
