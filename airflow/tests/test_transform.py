from __future__ import annotations

import json
from datetime import date, datetime, timedelta
from pathlib import Path
from typing import Any

import pytest
from pociag_processing import transform
from pociag_processing.transform import WARSAW, align_to_actual, parse_clock, parse_local_datetime

FIXTURES = Path(__file__).parent / "fixtures"


def load(name: str) -> Any:
    return json.loads((FIXTURES / name).read_text(encoding="utf-8"))


# ── Value parsing ────────────────────────────────────────────────────────────


@pytest.mark.parametrize(
    ("value", "day", "expected"),
    [
        ("06:16:00", None, timedelta(hours=6, minutes=16)),
        ("23:20:36", 0, timedelta(hours=23, minutes=20, seconds=36)),
        ("00:02:00", 1, timedelta(days=1, minutes=2)),
        ("23:50:00", -1, timedelta(days=-1, hours=23, minutes=50)),
        ("1.02:03:04", None, timedelta(days=1, hours=2, minutes=3, seconds=4)),
        ("07:05", None, timedelta(hours=7, minutes=5)),
        (None, 1, None),
        ("", None, None),
        ("PT8H", None, None),  # not a PLK clock: logged and dropped
    ],
)
def test_parse_clock(value: str | None, day: int | None, expected: timedelta | None) -> None:
    assert parse_clock(value, day) == expected


def test_parse_local_datetime_treats_naive_as_warsaw() -> None:
    summer = parse_local_datetime("2026-07-01T12:00:00")
    winter = parse_local_datetime("2026-12-01T12:00:00")
    assert summer == datetime(2026, 7, 1, 10, 0, tzinfo=WARSAW).replace(hour=12)
    assert summer is not None and summer.utcoffset() == timedelta(hours=2)
    assert winter is not None and winter.utcoffset() == timedelta(hours=1)
    assert parse_local_datetime("2026-07-01T12:00:00Z") == datetime.fromisoformat(
        "2026-07-01T12:00:00+00:00"
    )
    assert parse_local_datetime(None) is None


def test_align_to_actual_restores_missing_day_rollover() -> None:
    planned = datetime(2026, 10, 5, 0, 1, tzinfo=WARSAW)
    actual = datetime(2026, 10, 6, 0, 3, tzinfo=WARSAW)
    assert align_to_actual(planned, actual) == datetime(2026, 10, 6, 0, 1, tzinfo=WARSAW)


def test_align_to_actual_keeps_real_delays_and_missing_values() -> None:
    planned = datetime(2026, 10, 5, 8, 0, tzinfo=WARSAW)
    delayed = planned + timedelta(hours=3)
    assert align_to_actual(planned, delayed) == planned
    assert align_to_actual(planned, None) == planned
    assert align_to_actual(None, delayed) is None


# ── Stations ─────────────────────────────────────────────────────────────────


def test_stations_and_cities() -> None:
    stations = transform.stations(load("stations.json"))
    assert [(s.id, s.name) for s in stations] == [
        (280416, "Adamowo"),
        (280388, "Aleksandrów"),
        (21501, "Aleksandrów Kujawski"),
    ]
    cities = transform.station_cities(load("cities.json"))
    assert cities[140163] == "Berlin"
    assert cities[62687] == "Częstochowa"
    assert len(cities) == 12


def test_station_coordinates_skip_incomplete_entries() -> None:
    payload = {
        "stations": [
            {"external_id": 1, "latitude": 52.1, "longitude": 21.0},
            {"external_id": 2, "latitude": None, "longitude": 21.0},
            {"name": "no id", "latitude": 50.0, "longitude": 19.0},
        ]
    }
    coordinates = transform.station_coordinates(payload)
    assert [(c.id, c.latitude, c.longitude) for c in coordinates] == [(1, 52.1, 21.0)]


def test_packaged_station_coordinates_parse() -> None:
    from pociag_processing.pipelines import load_station_coordinates

    assert len(transform.station_coordinates(load_station_coordinates())) > 1000


# ── Schedules ────────────────────────────────────────────────────────────────


def test_schedules_trains() -> None:
    batch = transform.schedules(load("schedules.json"))

    named, overnight = batch.trains
    assert (named.schedule_id, named.order_id) == (2026, 841919828)
    assert named.name == "STADTTORE-LINIE"
    assert named.number == "80860"
    assert named.category == "R"
    assert (named.carrier_code, named.carrier_name) == ("PR", "POLREGIO S.A.")
    assert named.operating_dates == (date(2026, 10, 4),)
    assert overnight.name is None
    assert overnight.carrier_name == "Koleje Dolnośląskie S.A."
    assert {s.id for s in batch.stations} >= {150748, 273, 60103, 59394}


def test_schedules_stops_are_renumbered_in_plk_order() -> None:
    batch = transform.schedules(load("schedules.json"))
    stops = [s for s in batch.stops if s.train == (2026, 841919828)]

    # PLK order numbers -3, -2, -1, 1, 4, 8, 9 become 1..7.
    assert [s.seq for s in stops] == [1, 2, 3, 4, 5, 6, 7]
    assert [s.station_id for s in stops][:3] == [150748, 150750, 150751]
    assert stops[0].arrival == timedelta(hours=6, minutes=16)
    assert stops[1].departure == timedelta(hours=6, minutes=18, seconds=30)
    assert stops[-1].departure is None
    assert (stops[-2].platform, stops[-2].track) == ("1", "5")
    assert stops[0].platform is None


def test_schedules_day_offset_after_midnight() -> None:
    batch = transform.schedules(load("schedules.json"))
    stops = [s for s in batch.stops if s.train == (2026, 379184374)]

    assert stops[0].arrival is None
    assert stops[0].departure == timedelta(hours=23, minutes=15)
    assert stops[-1].arrival == timedelta(days=1, minutes=2)


# ── Operations ───────────────────────────────────────────────────────────────


def operation_batch() -> transform.OperationBatch:
    return transform.operations([load("operations_page1.json"), load("operations_page2.json")])


def test_operations_runs_and_stations() -> None:
    batch = operation_batch()

    assert [(o.schedule_id, o.order_id, o.operating_date, o.status) for o in batch.operations] == [
        (2026, 140621539, date(2026, 10, 4), "P"),
        (2026, 546554298, date(2026, 10, 5), "S"),
    ]
    assert len(batch.stops) == 6 + 22
    assert {s.station_id for s in batch.stops} <= {s.id for s in batch.stations}


def test_operations_stop_fields() -> None:
    stops = {s.seq: s for s in operation_batch().stops if s.run[1] == 140621539}

    first, delayed, last = stops[1], stops[10], stops[22]
    assert first.planned_arrival is None and first.is_confirmed
    assert delayed.planned_arrival == datetime(2026, 10, 4, 16, 54, tzinfo=WARSAW)
    assert delayed.actual_arrival == datetime(2026, 10, 4, 16, 58, tzinfo=WARSAW)
    assert (delayed.arrival_delay, delayed.departure_delay) == (4, 4)
    assert last.arrival_delay == 9 and last.departure_delay is None
    assert not last.is_confirmed and not last.is_cancelled


def test_operations_overnight_planned_times_are_aligned() -> None:
    stops = {s.seq: s for s in operation_batch().stops if s.run[1] == 546554298}

    assert stops[9].planned_arrival == datetime(2026, 10, 5, 23, 58, tzinfo=WARSAW)
    # PLK sends 2026-10-05T00:01 planned / 2026-10-06T00:01 actual.
    assert stops[10].planned_arrival == datetime(2026, 10, 6, 0, 1, tzinfo=WARSAW)
    assert stops[22].planned_departure == stops[22].actual_departure


def test_operations_last_copy_of_a_run_wins() -> None:
    page = load("operations_page1.json")
    later = json.loads(json.dumps(page))
    later["trains"][0]["trainStatus"] = "C"
    later["trains"][0]["stations"] = later["trains"][0]["stations"][:2]

    batch = transform.operations([page, later])

    assert [o.status for o in batch.operations] == ["C"]
    assert len(batch.stops) == 2


def test_operations_skip_unknown_status() -> None:
    page = load("operations_page1.json")
    page["trains"][0]["trainStatus"] = "Z"
    assert transform.operations([page]).operations == []


# ── Disruptions ──────────────────────────────────────────────────────────────


def test_disruptions() -> None:
    batch = transform.disruptions(load("disruptions.json"))

    typed, _, untyped = batch.disruptions
    assert typed.id == 4
    assert typed.type_code == "utr_31"
    assert (typed.start_station_id, typed.end_station_id) == (73700, 73312)
    assert (typed.date_from, typed.date_to) == (date(2026, 10, 3), date(2026, 10, 4))
    assert typed.affected_trains == 2
    assert typed.message.startswith("Na odcinku od stacji Tychy")

    assert untyped.type_code is None
    assert untyped.start_station_id is None
    assert untyped.affected_trains == 6
    assert {73700, 73312} <= {s.id for s in batch.stations}


def test_disruption_message_that_is_a_type_code_is_described() -> None:
    payload = {
        "disruptions": [{"disruptionId": 7, "message": "utr_55", "affectedRoutes": []}],
        "disruptionTypes": {"utr_55": "Ograniczenie prędkości pociągu"},
    }
    [row] = transform.disruptions(payload).disruptions
    assert (row.type_code, row.message) == ("utr_55", "Ograniczenie prędkości pociągu")
    assert (row.date_from, row.affected_trains) == (None, 0)
