from __future__ import annotations

import re
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from dataclasses import dataclass
from datetime import UTC, date, datetime, timedelta
from typing import Any, cast

import asyncpg
from opentelemetry import trace

from processor.models import PipelineName


@dataclass(slots=True)
class ProcessingRunRow:
    id: int
    pipeline: PipelineName
    run_date: date
    status: str
    records_fetched: int | None
    records_upserted: int | None
    started_at: datetime
    completed_at: datetime | None
    error_message: str | None


@dataclass(slots=True)
class UpsertResult:
    records_read: int
    records_written: int


class Repository:
    def __init__(self, pool: asyncpg.Pool) -> None:
        self._pool = pool
        self._tracer = trace.get_tracer("pociag.processor")

    @asynccontextmanager
    async def _connection(self) -> AsyncIterator[asyncpg.Connection]:
        pool = cast(Any, self._pool)
        async with pool.acquire() as conn:
            yield cast(asyncpg.Connection, conn)

    async def _execute(self, conn: asyncpg.Connection, query: str, *args: object) -> None:
        await cast(Any, conn).execute(query, *args)

    async def _fetch_value(
        self, conn: asyncpg.Connection, query: str, *args: object
    ) -> object | None:
        return cast(object | None, await cast(Any, conn).fetchval(query, *args))

    async def _fetch_int(self, conn: asyncpg.Connection, query: str, *args: object) -> int:
        value = await self._fetch_value(conn, query, *args)
        if value is None:
            return 0
        return int(cast(int, value))

    async def _fetch_required_int(
        self,
        conn: asyncpg.Connection,
        query: str,
        *args: object,
    ) -> int:
        value = await self._fetch_value(conn, query, *args)
        if value is None:
            raise RuntimeError("failed to create processing run")
        return int(cast(int, value))

    async def _fetch_rows(
        self,
        conn: asyncpg.Connection,
        query: str,
        *args: object,
    ) -> list[asyncpg.Record]:
        return cast(list[asyncpg.Record], await cast(Any, conn).fetch(query, *args))

    async def ping(self) -> None:
        with self._tracer.start_as_current_span("db.connection.ping"):
            async with self._connection() as conn:
                await self._execute(conn, "SELECT 1")

    async def is_pipeline_running(self, pipeline: PipelineName, run_date: date) -> bool:
        query = """
        SELECT EXISTS(
            SELECT 1
            FROM ingestion_runs
            WHERE pipeline = $1
              AND run_date = $2
              AND status = 'running'
        )
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.exists"):
            async with self._connection() as conn:
                value = await self._fetch_value(conn, query, pipeline, run_date)
                return bool(cast(bool | None, value))

    async def create_processing_run(self, pipeline: PipelineName, run_date: date) -> int:
        query = """
        INSERT INTO ingestion_runs (pipeline, run_date, status)
        VALUES ($1, $2, 'running')
        RETURNING id
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.insert"):
            async with self._connection() as conn:
                return await self._fetch_required_int(conn, query, pipeline, run_date)

    async def mark_processing_run_success(
        self,
        run_id: int,
        records_read: int,
        records_written: int,
    ) -> None:
        query = """
        UPDATE ingestion_runs
        SET status = 'success',
            records_fetched = $2,
            records_upserted = $3,
            completed_at = NOW(),
            updated_at = NOW()
        WHERE id = $1
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.mark_success"):
            async with self._connection() as conn:
                await self._execute(conn, query, run_id, records_read, records_written)

    async def mark_processing_run_failed(self, run_id: int, error_message: str) -> None:
        query = """
        UPDATE ingestion_runs
        SET status = 'failed',
            error_message = $2,
            completed_at = NOW(),
            updated_at = NOW()
        WHERE id = $1
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.mark_failed"):
            async with self._connection() as conn:
                await self._execute(conn, query, run_id, error_message)

    async def list_processing_runs(
        self,
        pipeline: PipelineName | None,
        limit: int,
    ) -> list[ProcessingRunRow]:
        query = """
        SELECT id,
               pipeline,
               run_date,
               status,
               records_fetched,
               records_upserted,
               started_at,
               completed_at,
               error_message
        FROM ingestion_runs
        WHERE ($1::text IS NULL OR pipeline = $1)
        ORDER BY started_at DESC
        LIMIT $2
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.list"):
            async with self._connection() as conn:
                rows = await self._fetch_rows(conn, query, pipeline, limit)
                return [self._map_run_row(row) for row in rows]

    def _map_run_row(self, row: asyncpg.Record) -> ProcessingRunRow:
        started_at = cast(datetime, row["started_at"])
        completed_at = cast(datetime | None, row["completed_at"])

        if started_at.tzinfo is None:
            started_at = started_at.replace(tzinfo=UTC)
        if completed_at is not None and completed_at.tzinfo is None:
            completed_at = completed_at.replace(tzinfo=UTC)

        return ProcessingRunRow(
            id=int(cast(int, row["id"])),
            pipeline=cast(PipelineName, row["pipeline"]),
            run_date=cast(date, row["run_date"]),
            status=cast(str, row["status"]),
            records_fetched=cast(int | None, row["records_fetched"]),
            records_upserted=cast(int | None, row["records_upserted"]),
            started_at=started_at,
            completed_at=completed_at,
            error_message=cast(str | None, row["error_message"]),
        )

    # ── Dictionary upserts ───────────────────────────────────────────────

    async def upsert_carriers(
        self, records: list[dict[str, Any]]
    ) -> UpsertResult:
        query = """
        INSERT INTO carriers (code, name, valid_from, valid_to)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (code) DO UPDATE
        SET name = EXCLUDED.name,
            valid_from = EXCLUDED.valid_from,
            valid_to = EXCLUDED.valid_to,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span("db.carriers.upsert"):
            async with self._connection() as conn:
                for rec in records:
                    await self._execute(
                        conn,
                        query,
                        rec.get("code"),
                        rec.get("name"),
                        _parse_timestamp(rec.get("validFrom")),
                        _parse_timestamp(rec.get("validTo")),
                    )
        return UpsertResult(
            records_read=len(records), records_written=len(records)
        )

    async def upsert_stations(
        self, records: list[dict[str, Any]]
    ) -> UpsertResult:
        query = """
        INSERT INTO stations (external_id, name, city)
        VALUES ($1, $2, $3)
        ON CONFLICT (external_id) DO UPDATE
        SET name = EXCLUDED.name,
            city = EXCLUDED.city,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span("db.stations.upsert"):
            async with self._connection() as conn:
                for rec in records:
                    await self._execute(
                        conn,
                        query,
                        rec.get("id"),
                        rec.get("name"),
                        rec.get("city"),
                    )
        return UpsertResult(
            records_read=len(records), records_written=len(records)
        )

    async def upsert_commercial_categories(
        self, records: list[dict[str, Any]]
    ) -> UpsertResult:
        query = """
        INSERT INTO commercial_categories
            (code, name, carrier_code, speed_category_code)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (code) DO UPDATE
        SET name = EXCLUDED.name,
            carrier_code = EXCLUDED.carrier_code,
            speed_category_code = EXCLUDED.speed_category_code,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span(
            "db.commercial_categories.upsert"
        ):
            async with self._connection() as conn:
                for rec in records:
                    await self._execute(
                        conn,
                        query,
                        rec.get("code"),
                        rec.get("name"),
                        rec.get("carrierCode"),
                        rec.get("speedCategoryCode"),
                    )
        return UpsertResult(
            records_read=len(records), records_written=len(records)
        )

    async def upsert_stop_types(
        self, records: list[dict[str, Any]]
    ) -> UpsertResult:
        query = """
        INSERT INTO stop_types (external_id, description)
        VALUES ($1, $2)
        ON CONFLICT (external_id) DO UPDATE
        SET description = EXCLUDED.description,
            updated_at = NOW()
        """
        with self._tracer.start_as_current_span("db.stop_types.upsert"):
            async with self._connection() as conn:
                for rec in records:
                    await self._execute(
                        conn,
                        query,
                        rec.get("id"),
                        rec.get("description"),
                    )
        return UpsertResult(
            records_read=len(records), records_written=len(records)
        )

    # ── Schedule upserts ─────────────────────────────────────────────────

    async def upsert_routes(
        self, routes: list[dict[str, Any]]
    ) -> UpsertResult:
        route_query = """
        INSERT INTO routes (
            schedule_id, order_id, train_order_id, name,
            carrier_code, national_number,
            international_arrival_num, international_departure_num,
            commercial_category_symbol
        )
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
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
        VALUES ($1, $2)
        ON CONFLICT (route_id, operating_date) DO NOTHING
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
            $1, $2, $3, $4, $5, $6, $7, $8, $9,
            $10, $11, $12, $13, $14, $15, $16, $17
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
        records_written = 0
        with self._tracer.start_as_current_span("db.routes.upsert"):
            async with self._connection() as conn:
                async with cast(Any, conn).transaction():
                    for route in routes:
                        route_id = await self._fetch_required_int(
                            conn,
                            route_query,
                            route.get("scheduleId"),
                            route.get("orderId"),
                            route.get("trainOrderId"),
                            route.get("name"),
                            route.get("carrierCode"),
                            route.get("nationalNumber"),
                            route.get("internationalArrivalNumber"),
                            route.get("internationalDepartureNumber"),
                            route.get("commercialCategorySymbol"),
                        )
                        records_written += 1

                        for od in route.get("operatingDates") or []:
                            await self._execute(
                                conn, date_query, route_id, _parse_date(od),
                            )

                        for stn in route.get("stations") or []:
                            await self._execute(
                                conn,
                                station_query,
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
                            )
        return UpsertResult(
            records_read=len(routes), records_written=records_written
        )


_ISO_DUR = re.compile(
    r"^P(?:(?P<days>\d+)D)?"
    r"(?:T(?:(?P<hours>\d+)H)?(?:(?P<minutes>\d+)M)?(?:(?P<seconds>\d+)S)?)?$",
)


def _parse_interval(value: str | None) -> timedelta | None:
    """Convert ISO 8601 duration like 'PT10H30M' to a timedelta."""
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
    """Convert an ISO 8601 date string to a date object."""
    return date.fromisoformat(value)


def _parse_timestamp(value: str | None) -> datetime | None:
    """Convert an ISO 8601 datetime string to a datetime object."""
    if value is None:
        return None
    return datetime.fromisoformat(value)
