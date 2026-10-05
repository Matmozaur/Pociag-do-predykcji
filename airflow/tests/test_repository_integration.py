"""Repository SQL against a real PostgreSQL with db/migrations applied.

Skipped unless POCIAG_TEST_DATABASE_URL points at a disposable database, e.g.
``postgresql://pociag:secret@localhost:5434/pociag_test``. The public schema is recreated.
"""

from __future__ import annotations

import json
import os
from collections.abc import Iterator
from dataclasses import replace
from datetime import date
from pathlib import Path
from typing import Any
from unittest.mock import patch

import psycopg2
import pytest
from pociag_processing import transform
from pociag_processing.models import Station, StationCoordinates
from pociag_processing.repository import Repository

DATABASE_URL = os.environ.get("POCIAG_TEST_DATABASE_URL")
MIGRATIONS = Path(__file__).resolve().parents[2] / "db" / "migrations"
FIXTURES = Path(__file__).parent / "fixtures"

pytestmark = pytest.mark.skipif(not DATABASE_URL, reason="POCIAG_TEST_DATABASE_URL not set")


def load(name: str) -> Any:
    return json.loads((FIXTURES / name).read_text(encoding="utf-8"))


@pytest.fixture
def db() -> Iterator[Any]:
    conn = psycopg2.connect(DATABASE_URL)
    conn.autocommit = True
    with conn.cursor() as cur:
        cur.execute("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
        cur.execute((MIGRATIONS / "001_init.up.sql").read_text())
    with patch("pociag_processing.repository.PostgresHook") as hook:
        hook.return_value.get_conn.side_effect = lambda: psycopg2.connect(DATABASE_URL)
        yield conn
    conn.close()


def query(conn: Any, sql: str) -> list[tuple[Any, ...]]:
    with conn.cursor() as cur:
        cur.execute(sql)
        return list(cur.fetchall())


def test_save_stations_sets_city_and_coordinates(db: Any) -> None:
    repo = Repository()
    stations = transform.stations(load("stations.json"))
    repo.save_stations(stations, {21501: "Aleksandrów"}, [StationCoordinates(21501, 52.87, 18.69)])
    repo.save_stations(stations, {}, [])  # names only: city/coordinates survive

    assert query(db, "SELECT id, name, city, latitude FROM stations ORDER BY id") == [
        (21501, "Aleksandrów Kujawski", "Aleksandrów", 52.87),
        (280388, "Aleksandrów", None, None),
        (280416, "Adamowo", None, None),
    ]


def test_save_schedules_is_idempotent_and_merges_operating_dates(db: Any) -> None:
    repo = Repository()
    batch = transform.schedules(load("schedules.json"))
    repo.save_schedules(batch)
    repo.save_schedules(batch)
    next_day = replace(batch, trains=[
        replace(t, operating_dates=(date(2026, 10, 5),)) for t in batch.trains
    ])
    repo.save_schedules(next_day)

    assert query(db, "SELECT count(*) FROM trains") == [(2,)]
    assert query(db, "SELECT count(*) FROM schedule_stops") == [(7 + 14,)]
    assert query(db, """
        SELECT name, number, category, carrier_name, operating_dates::text FROM trains
        WHERE order_id = 841919828
    """) == [("STADTTORE-LINIE", "80860", "R", "POLREGIO S.A.", "{2026-10-04,2026-10-05}")]
    assert query(db, """
        SELECT s.seq, s.station_id, s.arrival::text, s.departure::text FROM schedule_stops s
        JOIN trains t ON t.id = s.train_id WHERE t.order_id = 379184374 ORDER BY seq DESC LIMIT 1
    """) == [(14, 59394, "1 day 00:02:00", None)]


def test_save_operations_links_trains_and_updates_in_place(db: Any) -> None:
    repo = Repository()
    repo.save_schedules(transform.schedules(load("schedules.json")))
    pages = [load("operations_page1.json"), load("operations_page2.json")]
    repo.save_operations(transform.operations(pages))
    repo.save_operations(transform.operations(pages))

    # A station missing from every dictionary gets a placeholder; a dictionary name replaces it.
    page = load("operations_page1.json")
    page["trains"][0]["stations"][0]["stationId"] = 999999
    repo.save_operations(transform.operations([page]))
    assert query(db, "SELECT name FROM stations WHERE id = 999999") == [("Stacja 999999",)]
    repo.save_stations([Station(999999, "Lwów")], {}, [])
    repo.save_operations(transform.operations([page]))
    assert query(db, "SELECT name FROM stations WHERE id = 999999") == [("Lwów",)]

    # Neither run is in the schedules fixture: both get a bare train row.
    assert query(db, "SELECT count(*) FROM trains WHERE name IS NULL AND number IS NULL") == [(2,)]
    assert query(db, "SELECT count(*), count(DISTINCT train_id) FROM operations") == [(2, 2)]
    assert query(db, "SELECT count(*) FROM operation_stops") == [(28,)]
    assert query(db, """
        SELECT to_char(planned_arrival AT TIME ZONE 'Europe/Warsaw', 'YYYY-MM-DD HH24:MI')
        FROM operation_stops s JOIN operations o ON o.id = s.operation_id
        JOIN trains t ON t.id = o.train_id WHERE t.order_id = 546554298 AND s.seq = 10
    """) == [("2026-10-06 00:01",)]

    pages[0]["trains"][0]["trainStatus"] = "C"
    pages[0]["trains"][0]["stations"][-1]["isConfirmed"] = True
    repo.save_operations(transform.operations(pages))

    assert query(db, """
        SELECT o.status, s.is_confirmed FROM operations o
        JOIN operation_stops s ON s.operation_id = o.id
        JOIN trains t ON t.id = o.train_id WHERE t.order_id = 140621539 AND s.seq = 22
    """) == [("C", True)]


def test_replace_disruptions_replaces_the_snapshot(db: Any) -> None:
    repo = Repository()
    batch = transform.disruptions(load("disruptions.json"))
    repo.replace_disruptions(batch)
    repo.replace_disruptions(replace(batch, disruptions=batch.disruptions[:1]))

    assert query(db, """
        SELECT id, type_code, start_station_id, date_from::text, date_to::text, affected_trains
        FROM disruptions
    """) == [(4, "utr_31", 73700, "2026-10-03", "2026-10-04", 2)]


def test_down_migration_drops_everything(db: Any) -> None:
    with db.cursor() as cur:
        cur.execute((MIGRATIONS / "001_init.down.sql").read_text())
    assert query(db, "SELECT count(*) FROM pg_tables WHERE schemaname = 'public'") == [(0,)]
