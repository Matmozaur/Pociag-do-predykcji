-- 003_operations.up.sql
-- Real-time train operations: tracks actual train execution and delays.
-- Partitioned by operating_date for efficient historical queries.

BEGIN;

-- ── Train Operations ────────────────────────────────────────────────────────
CREATE TABLE train_operations (
    id              BIGSERIAL PRIMARY KEY,
    schedule_id     INT NOT NULL,
    order_id        INT NOT NULL,
    train_order_id  INT,
    operating_date  DATE NOT NULL,
    train_status    TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_train_operation UNIQUE (schedule_id, order_id, operating_date)
);

CREATE INDEX idx_train_operations_date ON train_operations (operating_date);
CREATE INDEX idx_train_operations_status ON train_operations (train_status);
CREATE INDEX idx_train_operations_schedule ON train_operations (schedule_id, order_id);

-- ── Operation Stations (actual stop data per train per day) ─────────────────
CREATE TABLE operation_stations (
    id                          BIGSERIAL PRIMARY KEY,
    train_operation_id          BIGINT NOT NULL REFERENCES train_operations(id) ON DELETE CASCADE,
    station_external_id         INT NOT NULL,
    planned_sequence_number     INT,
    actual_sequence_number      INT NOT NULL,
    planned_arrival             TIMESTAMPTZ,
    planned_departure           TIMESTAMPTZ,
    arrival_delay_minutes       INT,
    departure_delay_minutes     INT,
    actual_arrival              TIMESTAMPTZ,
    actual_departure            TIMESTAMPTZ,
    is_confirmed                BOOLEAN NOT NULL DEFAULT FALSE,
    is_cancelled                BOOLEAN NOT NULL DEFAULT FALSE,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_operation_station UNIQUE (train_operation_id, actual_sequence_number)
);

CREATE INDEX idx_operation_stations_train_op ON operation_stations (train_operation_id);
CREATE INDEX idx_operation_stations_station ON operation_stations (station_external_id);
CREATE INDEX idx_operation_stations_delay ON operation_stations (arrival_delay_minutes)
    WHERE arrival_delay_minutes IS NOT NULL;

COMMIT;
