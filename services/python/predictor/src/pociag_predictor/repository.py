"""All SQL of the predictor. Read-only: Airflow is the only writer of the curated tables."""

from __future__ import annotations

from datetime import date

from psycopg_pool import ConnectionPool

from pociag_predictor.tracing import get_tracer

# PLK leaves the delay minutes out of an on-time stop, so a confirmed arrival without them is 0.
# A stop without a planned arrival (the origin) has no arrival to rate.
_ARRIVAL_DELAYS = """
    SELECT COALESCE(s.arrival_delay, 0)
    FROM operations o
    JOIN operation_stops s ON s.operation_id = o.id
    WHERE o.train_id = %s AND o.operating_date BETWEEN %s AND %s
      AND s.station_id = %s AND s.planned_arrival IS NOT NULL
      AND s.is_confirmed AND NOT s.is_cancelled
"""


class Repository:
    def __init__(self, dsn: str) -> None:
        self._pool = ConnectionPool(
            dsn,
            min_size=1,
            max_size=5,
            # The session itself refuses writes, whatever the role is allowed to do.
            kwargs={"autocommit": True, "options": "-c default_transaction_read_only=on"},
            open=True,
        )

    def close(self) -> None:
        self._pool.close()

    def ping(self) -> None:
        with self._pool.connection(timeout=2) as conn:
            conn.execute("SELECT 1")

    def arrival_delays(
        self, train_id: int, station_id: int, first_day: date, last_day: date
    ) -> list[int]:
        with (
            get_tracer().start_as_current_span("db.operation_stops.arrival_delays"),
            self._pool.connection() as conn,
        ):
            rows = conn.execute(
                _ARRIVAL_DELAYS, (train_id, first_day, last_day, station_id)
            ).fetchall()
        return [int(row[0]) for row in rows]
