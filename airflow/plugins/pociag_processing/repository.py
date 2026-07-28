from __future__ import annotations

import logging
import re
from datetime import date, datetime, timedelta
from typing import Any

from airflow.providers.postgres.hooks.postgres import PostgresHook

from pociag_processing.models import PipelineName, UpsertResult
from pociag_processing.tracing import get_tracer

logger = logging.getLogger(__name__)

_ISO_DUR = re.compile(
    r"^P(?:(?P<days>\d+)D)?"
    r"(?:T(?:(?P<hours>\d+)H)?(?:(?P<minutes>\d+)M)?(?:(?P<seconds>\d+)S)?)?$",
)


def _parse_interval(value: str | None) -> timedelta | None:
    if value is None:
        return None
    m = _ISO_DUR.match(value)
    if m is None:
        return None
    return timedelta(
        days=int(m.group("days") or 0),
        hours=int(m.group("hours") or 0),
        minutes=int(m.group("minutes") or 0),
        seconds=int(m.group("seconds") or 0),
    )


def _parse_date(value: str) -> date:
    return date.fromisoformat(value)


def _parse_timestamp(value: str | None) -> datetime | None:
    if value is None:
        return None
    return datetime.fromisoformat(value)


class SyncRepository:
    def __init__(self, conn_id: str = "pociag_postgres") -> None:
        self._conn_id = conn_id
        self._tracer = get_tracer()

    def _get_conn(self) -> Any:
        hook = PostgresHook(postgres_conn_id=self._conn_id)
        return hook.get_conn()

    def ping(self) -> None:
        with self._tracer.start_as_current_span("db.connection.ping"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    cur.execute("SELECT 1")
            finally:
                conn.close()

    def is_pipeline_running(self, pipeline: PipelineName, run_date: date) -> bool:
        query = """
        SELECT EXISTS(
            SELECT 1
            FROM ingestion_runs
            WHERE pipeline = %s
              AND run_date = %s
              AND status = 'running'
        )
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.exists"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    cur.execute(query, (pipeline, run_date))
                    row = cur.fetchone()
                    return bool(row[0]) if row else False
            finally:
                conn.close()

    def create_processing_run(self, pipeline: PipelineName, run_date: date) -> int:
        query = """
        INSERT INTO ingestion_runs (pipeline, run_date, status)
        VALUES (%s, %s, 'running')
        RETURNING id
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.insert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    cur.execute(query, (pipeline, run_date))
                    row = cur.fetchone()
                    conn.commit()
                    if row is None:
                        raise RuntimeError("failed to create processing run")
                    return int(row[0])
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()

    def mark_processing_run_success(
        self,
        run_id: int,
        records_read: int,
        records_written: int,
    ) -> None:
        query = """
        UPDATE ingestion_runs
        SET status = 'success',
            records_fetched = %s,
            records_upserted = %s,
            completed_at = NOW(),
            updated_at = NOW()
        WHERE id = %s
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.mark_success"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    cur.execute(query, (records_read, records_written, run_id))
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()

    def mark_processing_run_failed(self, run_id: int, error_message: str) -> None:
        query = """
        UPDATE ingestion_runs
        SET status = 'failed',
            error_message = %s,
            completed_at = NOW(),
            updated_at = NOW()
        WHERE id = %s
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.mark_failed"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    cur.execute(query, (error_message, run_id))
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()

    def list_processing_runs(
        self,
        pipeline: PipelineName | None,
        limit: int,
    ) -> list[dict[str, Any]]:
        query = """
        SELECT id, pipeline, run_date, status, records_fetched,
               records_upserted, started_at, completed_at, error_message
        FROM ingestion_runs
        WHERE (%s::text IS NULL OR pipeline = %s)
        ORDER BY started_at DESC
        LIMIT %s
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.list"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    cur.execute(query, (pipeline, pipeline, limit))
                    rows = cur.fetchall()
                    return [
                        {
                            "id": row[0],
                            "pipeline": row[1],
                            "run_date": row[2],
                            "status": row[3],
                            "records_fetched": row[4],
                            "records_upserted": row[5],
                            "started_at": row[6],
                            "completed_at": row[7],
                            "error_message": row[8],
                        }
                        for row in rows
                    ]
            finally:
                conn.close()

    # ── Dictionary upserts ───────────────────────────────────────────────

    def upsert_carriers(self, records: list[dict[str, Any]]) -> UpsertResult:
        query = """
        INSERT INTO carriers (code, name, valid_from, valid_to)
        VALUES (%s, %s, %s, %s)
        ON CONFLICT (code) DO UPDATE
        SET name = EXCLUDED.name,
            valid_from = EXCLUDED.valid_from,
            valid_to = EXCLUDED.valid_to,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span("db.carriers.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for rec in records:
                        cur.execute(
                            query,
                            (
                                rec.get("code"),
                                rec.get("name"),
                                _parse_timestamp(rec.get("validFrom")),
                                _parse_timestamp(rec.get("validTo")),
                            ),
                        )
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(records_read=len(records), records_written=len(records))

    def upsert_stations(self, records: list[dict[str, Any]]) -> UpsertResult:
        query = """
        INSERT INTO stations (external_id, name, city)
        VALUES (%s, %s, %s)
        ON CONFLICT (external_id) DO UPDATE
        SET name = EXCLUDED.name,
            city = EXCLUDED.city,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span("db.stations.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for rec in records:
                        cur.execute(
                            query,
                            (rec.get("id"), rec.get("name"), rec.get("city")),
                        )
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(records_read=len(records), records_written=len(records))

    def upsert_station_cities(self, records: list[dict[str, Any]]) -> UpsertResult:
        query = """
        UPDATE stations
        SET city = %s,
            updated_at = NOW()
        WHERE external_id = %s
        """
        records_read = 0
        records_written = 0
        with self._tracer.start_as_current_span("db.stations.cities.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for city in records:
                        city_name = city.get("name")
                        if not isinstance(city_name, str) or not city_name.strip():
                            logger.warning("Skipping city record without a nonempty name")
                            continue
                        station_ids: list[int] = city.get("stationIds") or []
                        records_read += len(station_ids)
                        for station_id in station_ids:
                            cur.execute(query, (city_name, station_id))
                            records_written += cur.rowcount
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(records_read=records_read, records_written=records_written)

    def upsert_commercial_categories(self, records: list[dict[str, Any]]) -> UpsertResult:
        query = """
        INSERT INTO commercial_categories
            (code, name, carrier_code, speed_category_code)
        VALUES (%s, %s, %s, %s)
        ON CONFLICT (code) DO UPDATE
        SET name = EXCLUDED.name,
            carrier_code = EXCLUDED.carrier_code,
            speed_category_code = EXCLUDED.speed_category_code,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span("db.commercial_categories.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for rec in records:
                        cur.execute(
                            query,
                            (
                                rec.get("code"),
                                rec.get("name"),
                                rec.get("carrierCode"),
                                rec.get("speedCategoryCode"),
                            ),
                        )
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(records_read=len(records), records_written=len(records))

    def upsert_stop_types(self, records: list[dict[str, Any]]) -> UpsertResult:
        query = """
        INSERT INTO stop_types (external_id, description)
        VALUES (%s, %s)
        ON CONFLICT (external_id) DO UPDATE
        SET description = EXCLUDED.description,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span("db.stop_types.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for rec in records:
                        cur.execute(
                            query,
                            (rec.get("id"), rec.get("description")),
                        )
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(records_read=len(records), records_written=len(records))

    # ── Schedule upserts ─────────────────────────────────────────────────

    def upsert_routes(self, routes: list[dict[str, Any]]) -> UpsertResult:
        route_query = """
        INSERT INTO routes (
            schedule_id, order_id, train_order_id, name,
            carrier_code, national_number,
            international_arrival_num, international_departure_num,
            commercial_category_symbol
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (schedule_id, order_id) DO UPDATE
        SET train_order_id = EXCLUDED.train_order_id,
            name = EXCLUDED.name,
            carrier_code = EXCLUDED.carrier_code,
            national_number = EXCLUDED.national_number,
            international_arrival_num = EXCLUDED.international_arrival_num,
            international_departure_num = EXCLUDED.international_departure_num,
            commercial_category_symbol = EXCLUDED.commercial_category_symbol,
            updated_at = NOW()
        RETURNING id
        """
        date_query = """
        INSERT INTO route_operating_dates (route_id, operating_date)
        VALUES (%s, %s)
        ON CONFLICT (route_id, operating_date) DO NOTHING
        """
        delete_operating_dates_query = """
        DELETE FROM route_operating_dates WHERE route_id = %s
        """
        delete_connections_query = """
        DELETE FROM route_connections WHERE route_id = %s
        """
        connection_query = """
        INSERT INTO route_connections (
            route_id, external_connection_id, type_code, type_name,
            station_external_id, wagon_numbers,
            train1_order_id, train1_station_order, train1_day_offset,
            train2_order_id, train2_station_order, train2_day_offset
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        RETURNING id
        """
        connection_date_query = """
        INSERT INTO route_connection_operating_dates
            (route_connection_id, operating_date)
        VALUES (%s, %s)
        ON CONFLICT (route_connection_id, operating_date) DO NOTHING
        """
        station_query = """
        INSERT INTO route_stations (
            route_id, station_external_id, order_number,
            arrival_commercial_category, arrival_train_number,
            arrival_platform, arrival_track, arrival_day,
            arrival_time,
            departure_commercial_category, departure_train_number,
            departure_platform, departure_track, departure_day,
            departure_time,
            stop_type_id, stop_type_name
        )
        VALUES (
            %s, %s, %s, %s, %s, %s, %s, %s, %s,
            %s, %s, %s, %s, %s, %s, %s, %s
        )
        ON CONFLICT (route_id, order_number) DO UPDATE
        SET station_external_id = EXCLUDED.station_external_id,
            arrival_commercial_category = EXCLUDED.arrival_commercial_category,
            arrival_train_number = EXCLUDED.arrival_train_number,
            arrival_platform = EXCLUDED.arrival_platform,
            arrival_track = EXCLUDED.arrival_track,
            arrival_day = EXCLUDED.arrival_day,
            arrival_time = EXCLUDED.arrival_time,
            departure_commercial_category = EXCLUDED.departure_commercial_category,
            departure_train_number = EXCLUDED.departure_train_number,
            departure_platform = EXCLUDED.departure_platform,
            departure_track = EXCLUDED.departure_track,
            departure_day = EXCLUDED.departure_day,
            departure_time = EXCLUDED.departure_time,
            stop_type_id = EXCLUDED.stop_type_id,
            stop_type_name = EXCLUDED.stop_type_name,
            updated_at = NOW()
        """
        delete_stations_query = """
        DELETE FROM route_stations WHERE route_id = %s
        """
        records_written = 0
        with self._tracer.start_as_current_span("db.routes.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for route in routes:
                        cur.execute(
                            route_query,
                            (
                                route.get("scheduleId"),
                                route.get("orderId"),
                                route.get("trainOrderId"),
                                route.get("name"),
                                route.get("carrierCode"),
                                route.get("nationalNumber"),
                                route.get("internationalArrivalNumber"),
                                route.get("internationalDepartureNumber"),
                                route.get("commercialCategorySymbol"),
                            ),
                        )
                        row = cur.fetchone()
                        if row is None:
                            raise RuntimeError("failed to upsert route")
                        route_id = int(row[0])
                        records_written += 1

                        # A route detail is authoritative for every child collection.
                        cur.execute(delete_operating_dates_query, (route_id,))
                        operating_dates: list[str] = route.get("operatingDates") or []
                        for od in operating_dates:
                            cur.execute(date_query, (route_id, _parse_date(od)))

                        # The PLK payload is authoritative for a route, so replace its
                        # connection set. This keeps the table idempotent despite the
                        # legacy route_connections table having no natural-key constraint.
                        cur.execute(delete_connections_query, (route_id,))
                        connections: list[dict[str, Any]] = route.get("connections") or []
                        for connection in connections:
                            type_code = connection.get("typeCode")
                            if not isinstance(type_code, str) or not type_code.strip():
                                logger.warning(
                                    "Skipping route connection without a nonempty typeCode: route_id=%d connection_id=%r",
                                    route_id,
                                    connection.get("id"),
                                )
                                continue
                            cur.execute(
                                connection_query,
                                (
                                    route_id,
                                    connection.get("id"),
                                    type_code,
                                    connection.get("typeName"),
                                    connection.get("stationId"),
                                    connection.get("wagonNumbers"),
                                    connection.get("train1OrderId"),
                                    connection.get("train1StationOrder"),
                                    connection.get("train1DayOffset"),
                                    connection.get("train2OrderId"),
                                    connection.get("train2StationOrder"),
                                    connection.get("train2DayOffset"),
                                ),
                            )
                            row = cur.fetchone()
                            if row is None:
                                raise RuntimeError("failed to upsert route connection")
                            connection_id = int(row[0])
                            connection_dates: list[str] = connection.get("operatingDates") or []
                            for operating_date in connection_dates:
                                cur.execute(
                                    connection_date_query,
                                    (connection_id, _parse_date(operating_date)),
                                )

                        stations: list[dict[str, Any]] = route.get("stations") or []
                        cur.execute(delete_stations_query, (route_id,))
                        for stn in stations:
                            cur.execute(
                                station_query,
                                (
                                    route_id,
                                    stn.get("stationId"),
                                    stn.get("orderNumber"),
                                    stn.get("arrivalCommercialCategory"),
                                    stn.get("arrivalTrainNumber"),
                                    stn.get("arrivalPlatform"),
                                    stn.get("arrivalTrack"),
                                    stn.get("arrivalDay"),
                                    _parse_interval(stn.get("arrivalTime")),
                                    stn.get("departureCommercialCategory"),
                                    stn.get("departureTrainNumber"),
                                    stn.get("departurePlatform"),
                                    stn.get("departureTrack"),
                                    stn.get("departureDay"),
                                    _parse_interval(stn.get("departureTime")),
                                    stn.get("stopTypeId"),
                                    stn.get("stopTypeName"),
                                ),
                            )
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(records_read=len(routes), records_written=records_written)

    # ── Operation upserts ────────────────────────────────────────────────

    def upsert_operations(self, operations: list[dict[str, Any]]) -> UpsertResult:
        op_query = """
        INSERT INTO train_operations
            (schedule_id, order_id, train_order_id, operating_date, train_status)
        VALUES (%s, %s, %s, %s, %s)
        ON CONFLICT (schedule_id, order_id, operating_date) DO UPDATE
        SET train_order_id = EXCLUDED.train_order_id,
            train_status = EXCLUDED.train_status,
            updated_at = NOW()
        RETURNING id
        """
        station_query = """
        INSERT INTO operation_stations (
            train_operation_id, station_external_id,
            planned_sequence_number, actual_sequence_number,
            planned_arrival, planned_departure,
            arrival_delay_minutes, departure_delay_minutes,
            actual_arrival, actual_departure,
            is_confirmed, is_cancelled
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (train_operation_id, actual_sequence_number) DO UPDATE
        SET station_external_id = EXCLUDED.station_external_id,
            planned_sequence_number = EXCLUDED.planned_sequence_number,
            planned_arrival = EXCLUDED.planned_arrival,
            planned_departure = EXCLUDED.planned_departure,
            arrival_delay_minutes = EXCLUDED.arrival_delay_minutes,
            departure_delay_minutes = EXCLUDED.departure_delay_minutes,
            actual_arrival = EXCLUDED.actual_arrival,
            actual_departure = EXCLUDED.actual_departure,
            is_confirmed = EXCLUDED.is_confirmed,
            is_cancelled = EXCLUDED.is_cancelled,
            updated_at = NOW()
        """
        records_written = 0
        with self._tracer.start_as_current_span("db.train_operations.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for op in operations:
                        operating_date_value = op.get("operatingDate")
                        if not isinstance(operating_date_value, str):
                            raise ValueError("operation is missing operatingDate")
                        cur.execute(
                            op_query,
                            (
                                op.get("scheduleId"),
                                op.get("orderId"),
                                op.get("trainOrderId"),
                                _parse_date(operating_date_value),
                                op.get("trainStatus"),
                            ),
                        )
                        row = cur.fetchone()
                        if row is None:
                            raise RuntimeError("failed to upsert train operation")
                        op_id = int(row[0])
                        records_written += 1

                        stations: list[dict[str, Any]] = op.get("stations") or []
                        for stn in stations:
                            cur.execute(
                                station_query,
                                (
                                    op_id,
                                    stn.get("stationId"),
                                    stn.get("plannedSequenceNumber"),
                                    stn.get("actualSequenceNumber"),
                                    _parse_timestamp(stn.get("plannedArrival")),
                                    _parse_timestamp(stn.get("plannedDeparture")),
                                    stn.get("arrivalDelayMinutes"),
                                    stn.get("departureDelayMinutes"),
                                    _parse_timestamp(stn.get("actualArrival")),
                                    _parse_timestamp(stn.get("actualDeparture")),
                                    stn.get("isConfirmed", False),
                                    stn.get("isCancelled", False),
                                ),
                            )
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(records_read=len(operations), records_written=records_written)

    # ── Disruption upserts ───────────────────────────────────────────────

    def upsert_disruption_types(self, disruption_types: dict[str, str]) -> UpsertResult:
        query = """
        INSERT INTO disruption_types (code, name)
        VALUES (%s, %s)
        ON CONFLICT (code) DO UPDATE
        SET name = EXCLUDED.name,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span("db.disruption_types.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for code, name in disruption_types.items():
                        cur.execute(query, (code, name))
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(
            records_read=len(disruption_types), records_written=len(disruption_types)
        )

    def upsert_disruptions(self, disruptions: list[dict[str, Any]]) -> UpsertResult:
        disruption_query = """
        INSERT INTO disruptions (
            external_disruption_id, disruption_type_code,
            start_station_ext_id, end_station_ext_id,
            message, date_from, date_to
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s)
        ON CONFLICT (external_disruption_id) DO UPDATE
        SET disruption_type_code = EXCLUDED.disruption_type_code,
            start_station_ext_id = EXCLUDED.start_station_ext_id,
            end_station_ext_id = EXCLUDED.end_station_ext_id,
            message = EXCLUDED.message,
            date_from = EXCLUDED.date_from,
            date_to = EXCLUDED.date_to,
            updated_at = NOW()
        RETURNING id
        """
        delete_routes_query = """
        DELETE FROM disruption_affected_routes WHERE disruption_id = %s
        """
        route_query = """
        INSERT INTO disruption_affected_routes (
            disruption_id, schedule_id, order_id,
            train_order_id, operating_date, station_ext_id, sequence_number
        )
        VALUES (%s, %s, %s, %s, %s, %s, %s)
        """
        records_written = 0
        with self._tracer.start_as_current_span("db.disruptions.upsert"):
            conn = self._get_conn()
            try:
                with conn.cursor() as cur:
                    for d in disruptions:
                        cur.execute(
                            disruption_query,
                            (
                                d.get("disruptionId"),
                                d.get("disruptionTypeCode"),
                                d.get("startStationId"),
                                d.get("endStationId"),
                                d.get("message"),
                                _parse_date(d["dateFrom"]) if d.get("dateFrom") else None,
                                _parse_date(d["dateTo"]) if d.get("dateTo") else None,
                            ),
                        )
                        row = cur.fetchone()
                        if row is None:
                            raise RuntimeError("failed to upsert disruption")
                        disruption_id = int(row[0])
                        records_written += 1

                        cur.execute(delete_routes_query, (disruption_id,))

                        affected: list[dict[str, Any]] = d.get("affectedRoutes") or []
                        for ar in affected:
                            op_date = ar.get("operatingDate")
                            cur.execute(
                                route_query,
                                (
                                    disruption_id,
                                    ar.get("scheduleId"),
                                    ar.get("orderId"),
                                    ar.get("trainOrderId"),
                                    _parse_date(op_date) if op_date else None,
                                    ar.get("stationId"),
                                    ar.get("sequenceNumber"),
                                ),
                            )
                    conn.commit()
            except Exception:
                conn.rollback()
                raise
            finally:
                conn.close()
        return UpsertResult(records_read=len(disruptions), records_written=records_written)
