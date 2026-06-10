from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator, Iterator

import asyncpg
import pytest
from asgi_lifespan import LifespanManager
from httpx import ASGITransport, AsyncClient
from testcontainers.postgres import PostgresContainer

from processor.dependencies import get_settings


async def _setup_schema(database_dsn: str) -> None:
    conn = await asyncpg.connect(database_dsn)
    try:
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS ingestion_runs (
                id BIGSERIAL PRIMARY KEY,
                pipeline TEXT NOT NULL,
                run_date DATE NOT NULL,
                status TEXT NOT NULL DEFAULT 'running',
                records_fetched INT DEFAULT 0,
                records_upserted INT DEFAULT 0,
                started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                completed_at TIMESTAMPTZ,
                error_message TEXT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS raw_dictionaries (
                id BIGSERIAL PRIMARY KEY,
                fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                dictionary_type TEXT NOT NULL,
                payload JSONB NOT NULL,
                record_count INT,
                ingestion_run_id BIGINT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS raw_schedules (
                id BIGSERIAL PRIMARY KEY,
                fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                date_from DATE NOT NULL,
                date_to DATE NOT NULL,
                page INT NOT NULL DEFAULT 1,
                payload JSONB NOT NULL,
                record_count INT,
                ingestion_run_id BIGINT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS raw_operations (
                id BIGSERIAL PRIMARY KEY,
                fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                operating_date DATE NOT NULL,
                page INT NOT NULL DEFAULT 1,
                payload JSONB NOT NULL,
                record_count INT,
                ingestion_run_id BIGINT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS raw_disruptions (
                id BIGSERIAL PRIMARY KEY,
                fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                date_from DATE NOT NULL,
                date_to DATE NOT NULL,
                payload JSONB NOT NULL,
                record_count INT,
                ingestion_run_id BIGINT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
    finally:
        await conn.close()


@pytest.fixture(scope="session")
def database_dsn() -> Iterator[str]:
    with PostgresContainer(
        "postgres:16-alpine",
        username="processor",
        password="processor",
        dbname="processor_test",
    ) as postgres:
        dsn = postgres.get_connection_url().replace("postgresql+psycopg2://", "postgresql://")
        asyncio.run(_setup_schema(dsn))
        yield dsn


@pytest.fixture(autouse=True)
async def reset_tables(database_dsn: str) -> AsyncIterator[None]:
    conn = await asyncpg.connect(database_dsn)
    try:
        await conn.execute("TRUNCATE ingestion_runs RESTART IDENTITY CASCADE")
        await conn.execute("TRUNCATE raw_dictionaries RESTART IDENTITY CASCADE")
        await conn.execute("TRUNCATE raw_schedules RESTART IDENTITY CASCADE")
        await conn.execute("TRUNCATE raw_operations RESTART IDENTITY CASCADE")
        await conn.execute("TRUNCATE raw_disruptions RESTART IDENTITY CASCADE")
        yield
    finally:
        await conn.close()


@pytest.fixture
def app(database_dsn: str, monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setenv("DATABASE_DSN", database_dsn)
    get_settings.cache_clear()
    from processor.main import create_app

    return create_app()


@pytest.fixture
async def client(app) -> AsyncIterator[AsyncClient]:
    async with LifespanManager(app):
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://testserver") as async_client:
            yield async_client
