from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator, Iterator
from unittest.mock import MagicMock

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
        yield
    finally:
        await conn.close()


@pytest.fixture
def mock_lake_reader() -> MagicMock:
    lake = MagicMock()
    lake.count_raw_dictionaries.return_value = 100
    lake.count_raw_schedules.return_value = 8
    lake.count_raw_operations.return_value = 50
    lake.count_raw_disruptions.return_value = 10
    return lake


@pytest.fixture
def app(database_dsn: str, monkeypatch: pytest.MonkeyPatch):
    monkeypatch.setenv("DATABASE_DSN", database_dsn)
    monkeypatch.setenv("S3_ENDPOINT", "http://localhost:9000")
    monkeypatch.setenv("S3_BUCKET", "test-bucket")
    monkeypatch.setenv("S3_ACCESS_KEY", "test")
    monkeypatch.setenv("S3_SECRET_KEY", "test")
    get_settings.cache_clear()
    from processor.main import create_app

    application = create_app()
    return application


@pytest.fixture
async def client(app, mock_lake_reader: MagicMock) -> AsyncIterator[AsyncClient]:
    async with LifespanManager(app):
        app.state.lake_reader = mock_lake_reader
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://testserver") as async_client:
            yield async_client
