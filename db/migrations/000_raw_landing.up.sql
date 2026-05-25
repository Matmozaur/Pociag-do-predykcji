-- 000_raw_landing.up.sql
-- Raw landing tables for the ELT pipeline.
-- The Collector service writes exact PLK API responses here as JSONB.
-- The Processor service reads from these tables and writes to curated domain tables.
-- Raw data is immutable (append-only) and serves as reprocessing source.

BEGIN;

-- Required extension for trigram text search (used later in domain tables)
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ── Raw Schedules ───────────────────────────────────────────────────────────
-- Stores full PLK /schedules response payloads
CREATE TABLE raw_schedules (
    id              BIGSERIAL PRIMARY KEY,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    date_from       DATE NOT NULL,
    date_to         DATE NOT NULL,
    page            INT NOT NULL DEFAULT 1,
    payload         JSONB NOT NULL,
    record_count    INT,
    ingestion_run_id BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_raw_schedules_fetched ON raw_schedules (fetched_at DESC);
CREATE INDEX idx_raw_schedules_dates ON raw_schedules (date_from, date_to);

-- ── Raw Operations ──────────────────────────────────────────────────────────
-- Stores full PLK /operations response payloads
CREATE TABLE raw_operations (
    id              BIGSERIAL PRIMARY KEY,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    operating_date  DATE NOT NULL,
    page            INT NOT NULL DEFAULT 1,
    payload         JSONB NOT NULL,
    record_count    INT,
    ingestion_run_id BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_raw_operations_fetched ON raw_operations (fetched_at DESC);
CREATE INDEX idx_raw_operations_date ON raw_operations (operating_date);

-- ── Raw Disruptions ─────────────────────────────────────────────────────────
-- Stores full PLK /disruptions response payloads
CREATE TABLE raw_disruptions (
    id              BIGSERIAL PRIMARY KEY,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    date_from       DATE NOT NULL,
    date_to         DATE NOT NULL,
    payload         JSONB NOT NULL,
    record_count    INT,
    ingestion_run_id BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_raw_disruptions_fetched ON raw_disruptions (fetched_at DESC);
CREATE INDEX idx_raw_disruptions_dates ON raw_disruptions (date_from, date_to);

-- ── Raw Dictionaries ────────────────────────────────────────────────────────
-- Stores PLK /dictionaries/* response payloads
CREATE TABLE raw_dictionaries (
    id              BIGSERIAL PRIMARY KEY,
    fetched_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    dictionary_type TEXT NOT NULL,
    payload         JSONB NOT NULL,
    record_count    INT,
    ingestion_run_id BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_raw_dictionaries_fetched ON raw_dictionaries (fetched_at DESC);
CREATE INDEX idx_raw_dictionaries_type ON raw_dictionaries (dictionary_type);

COMMIT;
