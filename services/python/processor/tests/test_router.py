from __future__ import annotations

import asyncpg
import pytest


@pytest.mark.asyncio
async def test_process_schedules_success_returns_result(client, database_dsn: str) -> None:
    conn = await asyncpg.connect(database_dsn)
    try:
        await conn.execute(
            """
            INSERT INTO raw_schedules (date_from, date_to, payload, record_count, ingestion_run_id)
            VALUES
                ('2026-06-01', '2026-06-07', '{}'::jsonb, 5, 101),
                ('2026-06-03', '2026-06-06', '{}'::jsonb, 3, 101)
            """
        )
    finally:
        await conn.close()

    response = await client.post(
        "/api/v1/process/schedules",
        json={"date_from": "2026-06-01", "date_to": "2026-06-07"},
    )

    assert response.status_code == 200
    payload = response.json()
    assert payload["pipeline"] == "schedules"
    assert payload["status"] == "success"
    assert payload["records_read"] == 8
    assert payload["records_written"] == 8


@pytest.mark.asyncio
async def test_process_schedules_invalid_range_returns_bad_request(client) -> None:
    response = await client.post(
        "/api/v1/process/schedules",
        json={"date_from": "2026-06-10", "date_to": "2026-06-01"},
    )

    assert response.status_code == 400
    payload = response.json()
    assert payload["error"] == "invalid_request"


@pytest.mark.asyncio
async def test_process_operations_running_conflict_returns_conflict(
    client,
    database_dsn: str,
) -> None:
    conn = await asyncpg.connect(database_dsn)
    try:
        await conn.execute(
            """
            INSERT INTO ingestion_runs (pipeline, run_date, status)
            VALUES ('operations', '2026-06-01', 'running')
            """
        )
    finally:
        await conn.close()

    response = await client.post(
        "/api/v1/process/operations",
        json={"date": "2026-06-01"},
    )

    assert response.status_code == 409
    payload = response.json()
    assert payload["error"] == "already_running"


@pytest.mark.asyncio
async def test_get_processing_status_returns_runs(client, database_dsn: str) -> None:
    conn = await asyncpg.connect(database_dsn)
    try:
        await conn.execute(
            """
            INSERT INTO ingestion_runs (
                pipeline,
                run_date,
                status,
                records_fetched,
                records_upserted,
                completed_at
            )
            VALUES ('schedules', '2026-06-01', 'success', 10, 10, NOW())
            """
        )
    finally:
        await conn.close()

    response = await client.get(
        "/api/v1/process/status",
        params={"pipeline": "schedules", "limit": 1},
    )

    assert response.status_code == 200
    payload = response.json()
    assert len(payload["runs"]) == 1
    assert payload["runs"][0]["pipeline"] == "schedules"
