from __future__ import annotations

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from dataclasses import dataclass
from datetime import UTC, date, datetime
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
