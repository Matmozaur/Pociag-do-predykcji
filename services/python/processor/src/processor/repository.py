from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, date, datetime

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


class Repository:
    def __init__(self, pool: asyncpg.Pool) -> None:
        self._pool = pool
        self._tracer = trace.get_tracer("pociag.processor")

    async def ping(self) -> None:
        with self._tracer.start_as_current_span("db.connection.ping"):
            async with self._pool.acquire() as conn:
                await conn.execute("SELECT 1")

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
            async with self._pool.acquire() as conn:
                return bool(await conn.fetchval(query, pipeline, run_date))

    async def create_processing_run(self, pipeline: PipelineName, run_date: date) -> int:
        query = """
        INSERT INTO ingestion_runs (pipeline, run_date, status)
        VALUES ($1, $2, 'running')
        RETURNING id
        """
        with self._tracer.start_as_current_span("db.ingestion_runs.insert"):
            async with self._pool.acquire() as conn:
                run_id = await conn.fetchval(query, pipeline, run_date)
                if run_id is None:
                    raise RuntimeError("failed to create processing run")
                return int(run_id)

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
            async with self._pool.acquire() as conn:
                await conn.execute(query, run_id, records_read, records_written)

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
            async with self._pool.acquire() as conn:
                await conn.execute(query, run_id, error_message)

    async def count_raw_dictionaries(self, ingestion_run_id: int | None) -> int:
        with self._tracer.start_as_current_span("db.raw_dictionaries.count"):
            async with self._pool.acquire() as conn:
                if ingestion_run_id is not None:
                    return await self._count_dictionaries_for_run(conn, ingestion_run_id)

                latest_run_id = await conn.fetchval(
                    """
                    SELECT ingestion_run_id
                    FROM raw_dictionaries
                    WHERE ingestion_run_id IS NOT NULL
                    ORDER BY fetched_at DESC
                    LIMIT 1
                    """
                )
                if latest_run_id is None:
                    value = await conn.fetchval(
                        "SELECT COALESCE(SUM(record_count), 0) FROM raw_dictionaries"
                    )
                    return int(value or 0)
                return await self._count_dictionaries_for_run(conn, int(latest_run_id))

    async def _count_dictionaries_for_run(
        self,
        conn: asyncpg.Connection,
        ingestion_run_id: int,
    ) -> int:
        value = await conn.fetchval(
            "SELECT COALESCE(SUM(record_count), 0) "
            "FROM raw_dictionaries WHERE ingestion_run_id = $1",
            ingestion_run_id,
        )
        return int(value or 0)

    async def count_raw_schedules(
        self,
        date_from: date,
        date_to: date,
        ingestion_run_id: int | None,
    ) -> int:
        query_with_run = """
        SELECT COALESCE(SUM(record_count), 0)
        FROM raw_schedules
        WHERE ingestion_run_id = $1
        """
        query_with_dates = """
        SELECT COALESCE(SUM(record_count), 0)
        FROM raw_schedules
        WHERE date_from <= $2
          AND date_to >= $1
        """
        with self._tracer.start_as_current_span("db.raw_schedules.count"):
            async with self._pool.acquire() as conn:
                if ingestion_run_id is not None:
                    value = await conn.fetchval(query_with_run, ingestion_run_id)
                else:
                    value = await conn.fetchval(query_with_dates, date_from, date_to)
                return int(value or 0)

    async def count_raw_operations(self, operating_date: date, ingestion_run_id: int | None) -> int:
        query_with_run = """
        SELECT COALESCE(SUM(record_count), 0)
        FROM raw_operations
        WHERE ingestion_run_id = $1
        """
        query_with_date = """
        SELECT COALESCE(SUM(record_count), 0)
        FROM raw_operations
        WHERE operating_date = $1
        """
        with self._tracer.start_as_current_span("db.raw_operations.count"):
            async with self._pool.acquire() as conn:
                if ingestion_run_id is not None:
                    value = await conn.fetchval(query_with_run, ingestion_run_id)
                else:
                    value = await conn.fetchval(query_with_date, operating_date)
                return int(value or 0)

    async def count_raw_disruptions(
        self,
        date_from: date,
        date_to: date,
        ingestion_run_id: int | None,
    ) -> int:
        query_with_run = """
        SELECT COALESCE(SUM(record_count), 0)
        FROM raw_disruptions
        WHERE ingestion_run_id = $1
        """
        query_with_dates = """
        SELECT COALESCE(SUM(record_count), 0)
        FROM raw_disruptions
        WHERE date_from <= $2
          AND date_to >= $1
        """
        with self._tracer.start_as_current_span("db.raw_disruptions.count"):
            async with self._pool.acquire() as conn:
                if ingestion_run_id is not None:
                    value = await conn.fetchval(query_with_run, ingestion_run_id)
                else:
                    value = await conn.fetchval(query_with_dates, date_from, date_to)
                return int(value or 0)

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
            async with self._pool.acquire() as conn:
                rows = await conn.fetch(query, pipeline, limit)
                return [self._map_run_row(row) for row in rows]

    def _map_run_row(self, row: asyncpg.Record) -> ProcessingRunRow:
        started_at = row["started_at"]
        completed_at = row["completed_at"]

        if started_at.tzinfo is None:
            started_at = started_at.replace(tzinfo=UTC)
        if completed_at is not None and completed_at.tzinfo is None:
            completed_at = completed_at.replace(tzinfo=UTC)

        return ProcessingRunRow(
            id=int(row["id"]),
            pipeline=row["pipeline"],
            run_date=row["run_date"],
            status=row["status"],
            records_fetched=row["records_fetched"],
            records_upserted=row["records_upserted"],
            started_at=started_at,
            completed_at=completed_at,
            error_message=row["error_message"],
        )
