-- 005_ingestion_tracking.up.sql
-- Tracks ingestion pipeline runs for idempotency and observability.
-- Used by Airflow DAGs to determine when last successful pull occurred.

BEGIN;

-- ── Ingestion Runs ──────────────────────────────────────────────────────────
CREATE TABLE ingestion_runs (
    id              BIGSERIAL PRIMARY KEY,
    pipeline        TEXT NOT NULL,
    run_date        DATE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'running',
    records_fetched INT DEFAULT 0,
    records_upserted INT DEFAULT 0,
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_ingestion_status CHECK (status IN ('running', 'success', 'failed'))
);

CREATE INDEX idx_ingestion_runs_pipeline_date ON ingestion_runs (pipeline, run_date DESC);
CREATE INDEX idx_ingestion_runs_status ON ingestion_runs (status);

COMMIT;
