-- 004_disruptions.up.sql
-- Traffic disruptions and affected routes.

BEGIN;

-- ── Disruptions ─────────────────────────────────────────────────────────────
CREATE TABLE disruptions (
    id                      BIGSERIAL PRIMARY KEY,
    external_disruption_id  BIGINT NOT NULL UNIQUE,
    disruption_type_code    TEXT,
    start_station_ext_id    INT,
    end_station_ext_id      INT,
    message                 TEXT,
    date_from               DATE,
    date_to                 DATE,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_disruptions_dates ON disruptions (date_from, date_to);
CREATE INDEX idx_disruptions_type ON disruptions (disruption_type_code);

-- ── Disruption Affected Routes ──────────────────────────────────────────────
CREATE TABLE disruption_affected_routes (
    id                  BIGSERIAL PRIMARY KEY,
    disruption_id       BIGINT NOT NULL REFERENCES disruptions(id) ON DELETE CASCADE,
    schedule_id         INT NOT NULL,
    order_id            INT NOT NULL,
    train_order_id      INT,
    operating_date      DATE,
    station_ext_id      INT,
    sequence_number     INT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_disruption_affected_routes_disruption ON disruption_affected_routes (disruption_id);
CREATE INDEX idx_disruption_affected_routes_date ON disruption_affected_routes (operating_date);

COMMIT;
