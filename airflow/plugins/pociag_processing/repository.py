"""All SQL of the plugin. Each public method writes one batch in a single transaction and is
idempotent, so any sync can be re-run."""

from __future__ import annotations

from collections.abc import Iterable, Iterator, Sequence
from contextlib import contextmanager
from typing import Any

from airflow.providers.postgres.hooks.postgres import PostgresHook
from psycopg2.extras import execute_values

from pociag_processing.models import (
    DisruptionBatch,
    OperationBatch,
    RunKey,
    ScheduleBatch,
    Station,
    StationCoordinates,
    TrainKey,
)
from pociag_processing.tracing import get_tracer

PAGE_SIZE = 1000

_UPSERT_STATIONS = """
INSERT INTO stations (id, name) VALUES %s
ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, updated_at = now()
WHERE stations.name IS DISTINCT FROM EXCLUDED.name
"""

# PLK occasionally references stations missing from every dictionary (e.g. some foreign ones).
# They get a placeholder name, which a later dictionary entry replaces.
_INSERT_UNKNOWN_STATIONS = """
INSERT INTO stations (id, name) SELECT id, 'Stacja ' || id FROM (VALUES %s) AS v(id)
ON CONFLICT (id) DO NOTHING
"""

_SET_STATION_CITIES = """
UPDATE stations s SET city = v.city, updated_at = now()
FROM (VALUES %s) AS v(id, city)
WHERE s.id = v.id AND s.city IS DISTINCT FROM v.city
"""

_SET_STATION_COORDINATES = """
UPDATE stations s SET latitude = v.latitude, longitude = v.longitude, updated_at = now()
FROM (VALUES %s) AS v(id, latitude, longitude)
WHERE s.id = v.id
  AND (s.latitude, s.longitude) IS DISTINCT FROM (v.latitude, v.longitude)
"""

_UPSERT_TRAINS = """
INSERT INTO trains
    (schedule_id, order_id, name, number, category, carrier_code, carrier_name, operating_dates)
VALUES %s
ON CONFLICT (schedule_id, order_id) DO UPDATE SET
    name = EXCLUDED.name,
    number = EXCLUDED.number,
    category = EXCLUDED.category,
    carrier_code = EXCLUDED.carrier_code,
    carrier_name = EXCLUDED.carrier_name,
    operating_dates = ARRAY(
        SELECT DISTINCT d FROM unnest(trains.operating_dates || EXCLUDED.operating_dates) AS d
        ORDER BY d
    ),
    updated_at = now()
RETURNING id, schedule_id, order_id
"""
_UPSERT_TRAINS_TEMPLATE = "(%s, %s, %s, %s, %s, %s, %s, %s::date[])"

_DELETE_SCHEDULE_STOPS = "DELETE FROM schedule_stops WHERE train_id = ANY(%s)"

_INSERT_SCHEDULE_STOPS = """
INSERT INTO schedule_stops (train_id, seq, station_id, arrival, departure, platform, track)
VALUES %s
"""

_INSERT_BARE_TRAINS = """
INSERT INTO trains (schedule_id, order_id) VALUES %s
ON CONFLICT (schedule_id, order_id) DO NOTHING
"""

_SELECT_TRAIN_IDS = """
SELECT t.id, t.schedule_id, t.order_id
FROM trains t JOIN (VALUES %s) AS k(schedule_id, order_id) USING (schedule_id, order_id)
"""

_UPSERT_OPERATIONS = """
INSERT INTO operations (train_id, operating_date, status) VALUES %s
ON CONFLICT (train_id, operating_date) DO UPDATE SET status = EXCLUDED.status, updated_at = now()
RETURNING id, train_id, operating_date
"""

_UPSERT_OPERATION_STOPS = """
INSERT INTO operation_stops (
    operation_id, seq, station_id,
    planned_arrival, planned_departure, actual_arrival, actual_departure,
    arrival_delay, departure_delay, is_confirmed, is_cancelled
) VALUES %s
ON CONFLICT (operation_id, seq) DO UPDATE SET
    station_id = EXCLUDED.station_id,
    planned_arrival = EXCLUDED.planned_arrival,
    planned_departure = EXCLUDED.planned_departure,
    actual_arrival = EXCLUDED.actual_arrival,
    actual_departure = EXCLUDED.actual_departure,
    arrival_delay = EXCLUDED.arrival_delay,
    departure_delay = EXCLUDED.departure_delay,
    is_confirmed = EXCLUDED.is_confirmed,
    is_cancelled = EXCLUDED.is_cancelled
WHERE (
    operation_stops.station_id, operation_stops.planned_arrival, operation_stops.planned_departure,
    operation_stops.actual_arrival, operation_stops.actual_departure,
    operation_stops.arrival_delay, operation_stops.departure_delay,
    operation_stops.is_confirmed, operation_stops.is_cancelled
) IS DISTINCT FROM (
    EXCLUDED.station_id, EXCLUDED.planned_arrival, EXCLUDED.planned_departure,
    EXCLUDED.actual_arrival, EXCLUDED.actual_departure,
    EXCLUDED.arrival_delay, EXCLUDED.departure_delay,
    EXCLUDED.is_confirmed, EXCLUDED.is_cancelled
)
"""

_DELETE_DISRUPTIONS = "DELETE FROM disruptions"

_INSERT_DISRUPTIONS = """
INSERT INTO disruptions (
    id, type_code, start_station_id, end_station_id,
    message, date_from, date_to, affected_trains
) VALUES %s
"""


class Repository:
    def __init__(self, conn_id: str = "pociag_postgres") -> None:
        self._conn_id = conn_id
        self._tracer = get_tracer()

    @contextmanager
    def _transaction(self, span_name: str) -> Iterator[Any]:
        with self._tracer.start_as_current_span(span_name):
            conn: Any = PostgresHook(postgres_conn_id=self._conn_id).get_conn()
            try:
                with conn.cursor() as cur:
                    yield cur
                conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()

    def save_stations(
        self,
        stations: Sequence[Station],
        cities: dict[int, str],
        coordinates: Sequence[StationCoordinates],
    ) -> None:
        with self._transaction("db.stations.save") as cur:
            _upsert_stations(cur, stations)
            _values(cur, _SET_STATION_CITIES, sorted(cities.items()))
            coordinate_rows = [(c.id, c.latitude, c.longitude) for c in coordinates]
            _values(cur, _SET_STATION_COORDINATES, coordinate_rows)

    def save_schedules(self, batch: ScheduleBatch) -> None:
        """Upsert the trains and replace their planned stops."""
        with self._transaction("db.schedules.save") as cur:
            _upsert_stations(cur, batch.stations)
            _insert_unknown_stations(cur, {s.station_id for s in batch.stops})
            rows = [
                (t.schedule_id, t.order_id, t.name, t.number, t.category,
                 t.carrier_code, t.carrier_name, list(t.operating_dates))
                for t in batch.trains
            ]
            returned = _values(cur, _UPSERT_TRAINS, rows, template=_UPSERT_TRAINS_TEMPLATE)
            train_ids: dict[TrainKey, int] = {(r[1], r[2]): r[0] for r in returned}

            cur.execute(_DELETE_SCHEDULE_STOPS, (list(train_ids.values()),))
            _values(cur, _INSERT_SCHEDULE_STOPS, [
                (train_ids[s.train], s.seq, s.station_id,
                 s.arrival, s.departure, s.platform, s.track)
                for s in batch.stops
            ])

    def save_operations(self, batch: OperationBatch) -> None:
        """Upsert the runs (marking them seen now) and their stops; unchanged stops are skipped."""
        with self._transaction("db.operations.save") as cur:
            _upsert_stations(cur, batch.stations)
            _insert_unknown_stations(cur, {s.station_id for s in batch.stops})
            keys = sorted({(op.schedule_id, op.order_id) for op in batch.operations})
            _values(cur, _INSERT_BARE_TRAINS, keys)
            train_ids: dict[TrainKey, int] = {
                (r[1], r[2]): r[0] for r in _values(cur, _SELECT_TRAIN_IDS, keys)
            }

            returned = _values(cur, _UPSERT_OPERATIONS, [
                (train_ids[(op.schedule_id, op.order_id)], op.operating_date, op.status)
                for op in batch.operations
            ])
            key_by_train = {v: k for k, v in train_ids.items()}
            run_ids: dict[RunKey, int] = {
                (*key_by_train[r[1]], r[2]): r[0] for r in returned
            }

            _values(cur, _UPSERT_OPERATION_STOPS, [
                (run_ids[s.run], s.seq, s.station_id,
                 s.planned_arrival, s.planned_departure, s.actual_arrival, s.actual_departure,
                 s.arrival_delay, s.departure_delay, s.is_confirmed, s.is_cancelled)
                for s in batch.stops
            ])

    def replace_disruptions(self, batch: DisruptionBatch) -> None:
        with self._transaction("db.disruptions.replace") as cur:
            _upsert_stations(cur, batch.stations)
            _insert_unknown_stations(cur, {
                station_id for d in batch.disruptions
                for station_id in (d.start_station_id, d.end_station_id) if station_id is not None
            })
            cur.execute(_DELETE_DISRUPTIONS)
            _values(cur, _INSERT_DISRUPTIONS, [
                (d.id, d.type_code, d.start_station_id, d.end_station_id,
                 d.message, d.date_from, d.date_to, d.affected_trains)
                for d in batch.disruptions
            ])


def _upsert_stations(cur: Any, stations: Iterable[Station]) -> None:
    by_id = {s.id: s.name for s in stations}
    _values(cur, _UPSERT_STATIONS, sorted(by_id.items()))


def _insert_unknown_stations(cur: Any, station_ids: Iterable[int]) -> None:
    _values(cur, _INSERT_UNKNOWN_STATIONS, [(i,) for i in sorted(station_ids)])


def _values(
    cur: Any, sql: str, rows: Sequence[tuple[Any, ...]], template: str | None = None
) -> list[tuple[Any, ...]]:
    """``execute_values`` that skips empty batches and returns rows of a RETURNING/SELECT."""
    if not rows:
        return []
    fetch = "RETURNING" in sql or sql.lstrip().startswith("SELECT")
    result = execute_values(cur, sql, rows, template=template, page_size=PAGE_SIZE, fetch=fetch)
    return list(result) if fetch else []
