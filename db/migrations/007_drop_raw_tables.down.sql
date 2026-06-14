-- Re-create raw landing tables (restored from 000_raw_landing.up.sql)
-- These tables are no longer used; raw data now lands in MinIO as Parquet files.

BEGIN;

CREATE TABLE IF NOT EXISTS raw_dictionaries (
    id              BIGSERIAL PRIMARY KEY,
    dictionary_type TEXT NOT NULL,
    payload         JSONB NOT NULL,
    record_count    INTEGER NOT NULL DEFAULT 0,
    ingestion_run_id BIGINT REFERENCES ingestion_runs(id),
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS raw_schedules (
    id              BIGSERIAL PRIMARY KEY,
    date_from       DATE NOT NULL,
    date_to         DATE NOT NULL,
    page            INTEGER NOT NULL DEFAULT 1,
    payload         JSONB NOT NULL,
    record_count    INTEGER NOT NULL DEFAULT 0,
    ingestion_run_id BIGINT REFERENCES ingestion_runs(id),
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS raw_operations (
    id              BIGSERIAL PRIMARY KEY,
    operating_date  DATE NOT NULL,
    page            INTEGER NOT NULL DEFAULT 1,
    payload         JSONB NOT NULL,
    record_count    INTEGER NOT NULL DEFAULT 0,
    ingestion_run_id BIGINT REFERENCES ingestion_runs(id),
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS raw_disruptions (
    id              BIGSERIAL PRIMARY KEY,
    date_from       DATE NOT NULL,
    date_to         DATE NOT NULL,
    payload         JSONB NOT NULL,
    record_count    INTEGER NOT NULL DEFAULT 0,
    ingestion_run_id BIGINT REFERENCES ingestion_runs(id),
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
