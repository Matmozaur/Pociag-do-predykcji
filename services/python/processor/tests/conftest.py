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
        await conn.execute("CREATE EXTENSION IF NOT EXISTS pg_trgm")
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
            CREATE TABLE IF NOT EXISTS carriers (
                id BIGSERIAL PRIMARY KEY,
                code TEXT NOT NULL UNIQUE,
                name TEXT NOT NULL,
                valid_from TIMESTAMPTZ,
                valid_to TIMESTAMPTZ,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS stations (
                id BIGSERIAL PRIMARY KEY,
                external_id INT NOT NULL UNIQUE,
                name TEXT NOT NULL,
                city TEXT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS commercial_categories (
                id BIGSERIAL PRIMARY KEY,
                code TEXT NOT NULL UNIQUE,
                name TEXT NOT NULL,
                carrier_code TEXT,
                speed_category_code TEXT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS stop_types (
                id BIGSERIAL PRIMARY KEY,
                external_id INT NOT NULL UNIQUE,
                description TEXT NOT NULL,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS routes (
                id BIGSERIAL PRIMARY KEY,
                schedule_id INT NOT NULL,
                order_id INT NOT NULL,
                train_order_id INT,
                name TEXT,
                carrier_code TEXT,
                national_number TEXT,
                international_arrival_num TEXT,
                international_departure_num TEXT,
                commercial_category_symbol TEXT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                CONSTRAINT uq_routes_schedule_order
                    UNIQUE (schedule_id, order_id)
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS route_operating_dates (
                id BIGSERIAL PRIMARY KEY,
                route_id BIGINT NOT NULL REFERENCES routes(id)
                    ON DELETE CASCADE,
                operating_date DATE NOT NULL,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                CONSTRAINT uq_route_operating_date
                    UNIQUE (route_id, operating_date)
            )
            """
        )
        await conn.execute(
            """
            CREATE TABLE IF NOT EXISTS route_stations (
                id BIGSERIAL PRIMARY KEY,
                route_id BIGINT NOT NULL REFERENCES routes(id)
                    ON DELETE CASCADE,
                station_external_id INT NOT NULL,
                order_number INT NOT NULL,
                arrival_commercial_category TEXT,
                arrival_train_number TEXT,
                arrival_platform TEXT,
                arrival_track TEXT,
                arrival_day INT,
                arrival_time INTERVAL,
                departure_commercial_category TEXT,
                departure_train_number TEXT,
                departure_platform TEXT,
                departure_track TEXT,
                departure_day INT,
                departure_time INTERVAL,
                stop_type_id INT,
                stop_type_name TEXT,
                created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                CONSTRAINT uq_route_station_order
                    UNIQUE (route_id, order_number)
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
        await conn.execute(
            "TRUNCATE ingestion_runs, route_stations, "
            "route_operating_dates, routes, carriers, stations, "
            "commercial_categories, stop_types "
            "RESTART IDENTITY CASCADE"
        )
        yield
    finally:
        await conn.close()


@pytest.fixture
def mock_lake_reader() -> MagicMock:
    lake = MagicMock()
    lake.read_raw_dictionaries.return_value = [
        {
            "metadata": {"dictionary_type": "carriers", "record_count": "2"},
            "payload": {
                "carriers": [
                    {"code": "IC", "name": "PKP Intercity"},
                    {"code": "KM", "name": "Koleje Mazowieckie"},
                ],
            },
        },
        {
            "metadata": {"dictionary_type": "stations", "record_count": "1"},
            "payload": {
                "stations": [
                    {"id": 1001, "name": "Warszawa Centralna"},
                ],
            },
        },
    ]
    lake.read_raw_schedules.return_value = [
        {
            "metadata": {"record_count": "1"},
            "payload": {
                "routes": [
                    {
                        "scheduleId": 25,
                        "orderId": 100,
                        "name": "IC Test",
                        "carrierCode": "IC",
                        "stations": [
                            {
                                "stationId": 1001,
                                "orderNumber": 1,
                                "departureTime": "PT8H30M",
                            },
                        ],
                        "operatingDates": ["2026-06-01"],
                    },
                ],
            },
        },
    ]
    lake.read_raw_operations.return_value = []
    lake.read_raw_disruptions.return_value = []
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
