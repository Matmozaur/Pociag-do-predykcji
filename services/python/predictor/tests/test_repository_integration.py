"""SQL against a real PostgreSQL with db/migrations applied. Skipped unless
POCIAG_TEST_DATABASE_URL points at a disposable database; its public schema is recreated."""

from __future__ import annotations

import os
from collections.abc import Iterator
from datetime import date
from pathlib import Path

import psycopg
import pytest

from pociag_predictor.repository import Repository

DSN = os.environ.get("POCIAG_TEST_DATABASE_URL")
MIGRATIONS = Path(__file__).resolve().parents[4] / "db" / "migrations"

pytestmark = pytest.mark.skipif(not DSN, reason="POCIAG_TEST_DATABASE_URL not set")

# Train 10 arrives at station 1 as its stop 1 on each operating date (the origin is station 9).
SEED = """
INSERT INTO stations (id, name) VALUES (1, 'Warszawa Centralna'), (2, 'Kielce'), (9, 'Origin');
INSERT INTO trains (id, schedule_id, order_id) VALUES (10, 2026, 100), (11, 2026, 101);
INSERT INTO operations (id, train_id, operating_date, status) VALUES
    (110, 10, '2026-09-27', 'C'), -- before the window
    (111, 10, '2026-09-28', 'C'), -- first day of the window
    (112, 10, '2026-09-30', 'C'),
    (113, 10, '2026-10-01', 'C'),
    (114, 10, '2026-10-02', 'C'),
    (115, 10, '2026-10-05', 'S'), -- after the window
    (116, 10, '2026-10-03', 'C'),
    (117, 11, '2026-10-03', 'C'); -- another route
INSERT INTO operation_stops (operation_id, seq, station_id, planned_arrival, arrival_delay,
                             is_confirmed, is_cancelled) VALUES
    (110, 1, 1, '2026-09-27T11:08Z', 50, true, false),
    (111, 1, 1, '2026-09-28T11:08Z', 15, true, false),
    (112, 1, 1, '2026-09-30T11:08Z', NULL, true, false),  -- on time: PLK omits the minutes
    (113, 1, 1, '2026-10-01T11:08Z', -2, true, false),    -- early
    (114, 1, 1, '2026-10-02T11:08Z', 30, true, true),     -- cancelled stop
    (115, 1, 1, '2026-10-05T11:08Z', 30, true, false),
    (116, 1, 1, '2026-10-03T11:08Z', 40, false, false),   -- a forecast, not history
    (117, 1, 1, '2026-10-03T11:08Z', 8, true, false);
-- The origin has no planned arrival; another stop of the same run is another station.
INSERT INTO operation_stops (operation_id, seq, station_id, planned_departure, departure_delay,
                             is_confirmed) VALUES (111, 2, 9, '2026-09-28T11:00Z', 9, true);
INSERT INTO operation_stops (operation_id, seq, station_id, planned_arrival, arrival_delay,
                             is_confirmed) VALUES
    (111, 3, 9, '2026-09-28T11:30Z', 11, true),
    (111, 4, 2, '2026-09-28T12:00Z', 12, true);
"""


@pytest.fixture
def repository() -> Iterator[Repository]:
    assert DSN
    with psycopg.connect(DSN, autocommit=True) as conn:
        conn.execute("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
        for migration in sorted(MIGRATIONS.glob("*.up.sql")):
            conn.execute(migration.read_text())
        conn.execute(SEED)
    repo = Repository(DSN)
    yield repo
    repo.close()


def test_arrival_delays(repository: Repository) -> None:
    delays = repository.arrival_delays(10, 1, date(2026, 9, 28), date(2026, 10, 4))

    assert sorted(delays) == [-2, 0, 15]


def test_arrival_delays_of_another_route_and_station(repository: Repository) -> None:
    assert repository.arrival_delays(11, 1, date(2026, 9, 28), date(2026, 10, 4)) == [8]
    assert repository.arrival_delays(10, 2, date(2026, 9, 28), date(2026, 10, 4)) == [12]
    assert repository.arrival_delays(99, 1, date(2026, 9, 28), date(2026, 10, 4)) == []


def test_ping(repository: Repository) -> None:
    repository.ping()


def test_the_session_refuses_writes(repository: Repository) -> None:
    with (
        pytest.raises(psycopg.errors.ReadOnlySqlTransaction),
        repository._pool.connection() as conn,
    ):
        conn.execute("DELETE FROM operations")
