-- 001_dictionaries.up.sql
-- Reference/dictionary tables derived from PLK Open Data /dictionaries/ endpoints.
-- These are the foundation for all schedule and operation data.
-- Written by the Processor service after transforming raw_dictionaries data.

BEGIN;

-- ── Carriers ────────────────────────────────────────────────────────────────
CREATE TABLE carriers (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    valid_from  TIMESTAMPTZ,
    valid_to    TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_carriers_code ON carriers (code);

-- ── Stations ────────────────────────────────────────────────────────────────
CREATE TABLE stations (
    id              BIGSERIAL PRIMARY KEY,
    external_id     INT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    city            TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_stations_external_id ON stations (external_id);
CREATE INDEX idx_stations_name ON stations USING gin (name gin_trgm_ops);
CREATE INDEX idx_stations_city ON stations (city);

-- ── Commercial Categories ───────────────────────────────────────────────────
CREATE TABLE commercial_categories (
    id                  BIGSERIAL PRIMARY KEY,
    code                TEXT NOT NULL UNIQUE,
    name                TEXT NOT NULL,
    carrier_code        TEXT,
    speed_category_code TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Stop Types ──────────────────────────────────────────────────────────────
CREATE TABLE stop_types (
    id              BIGSERIAL PRIMARY KEY,
    external_id     INT NOT NULL UNIQUE,
    description     TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ── Train Statuses (enum-like reference) ────────────────────────────────────
CREATE TABLE train_statuses (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Seed known statuses from PLK API
INSERT INTO train_statuses (code, name) VALUES
    ('S', 'Not Started'),
    ('P', 'In Progress'),
    ('C', 'Completed'),
    ('X', 'Cancelled'),
    ('Q', 'Partial Cancelled');

-- ── Disruption Types ────────────────────────────────────────────────────────
CREATE TABLE disruption_types (
    id          BIGSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMIT;
