-- 002_schedules.up.sql
-- Schedule/timetable tables: routes with their stops and operating dates.
-- Designed for upsert semantics: (schedule_id, order_id) is the natural key.

BEGIN;

-- ── Routes (train route definitions) ────────────────────────────────────────
CREATE TABLE routes (
    id                          BIGSERIAL PRIMARY KEY,
    schedule_id                 INT NOT NULL,
    order_id                    INT NOT NULL,
    train_order_id              INT,
    name                        TEXT,
    carrier_code                TEXT,
    national_number             TEXT,
    international_arrival_num   TEXT,
    international_departure_num TEXT,
    commercial_category_symbol  TEXT,
    created_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_routes_schedule_order UNIQUE (schedule_id, order_id)
);

CREATE INDEX idx_routes_carrier_code ON routes (carrier_code);
CREATE INDEX idx_routes_name ON routes (name);

-- ── Route Operating Dates ───────────────────────────────────────────────────
CREATE TABLE route_operating_dates (
    id              BIGSERIAL PRIMARY KEY,
    route_id        BIGINT NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    operating_date  DATE NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_route_operating_date UNIQUE (route_id, operating_date)
);

CREATE INDEX idx_route_operating_dates_date ON route_operating_dates (operating_date);
CREATE INDEX idx_route_operating_dates_route_id ON route_operating_dates (route_id);

-- ── Route Stations (stops along a route) ────────────────────────────────────
CREATE TABLE route_stations (
    id                              BIGSERIAL PRIMARY KEY,
    route_id                        BIGINT NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    station_external_id             INT NOT NULL,
    order_number                    INT NOT NULL,
    arrival_commercial_category     TEXT,
    arrival_train_number            TEXT,
    arrival_platform                TEXT,
    arrival_track                   TEXT,
    arrival_day                     INT,
    arrival_time                    INTERVAL,
    departure_commercial_category   TEXT,
    departure_train_number          TEXT,
    departure_platform              TEXT,
    departure_track                 TEXT,
    departure_day                   INT,
    departure_time                  INTERVAL,
    stop_type_id                    INT,
    stop_type_name                  TEXT,
    created_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                      TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_route_station_order UNIQUE (route_id, order_number)
);

CREATE INDEX idx_route_stations_route_id ON route_stations (route_id);
CREATE INDEX idx_route_stations_station_ext_id ON route_stations (station_external_id);

-- ── Route Connections (train-to-train links) ────────────────────────────────
CREATE TABLE route_connections (
    id                      BIGSERIAL PRIMARY KEY,
    route_id                BIGINT NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
    external_connection_id  TEXT,
    type_code               TEXT,
    type_name               TEXT,
    station_external_id     INT,
    wagon_numbers           TEXT,
    train1_order_id         INT,
    train1_station_order    INT,
    train1_day_offset       INT,
    train2_order_id         INT,
    train2_station_order    INT,
    train2_day_offset       INT,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_route_connections_route_id ON route_connections (route_id);

COMMIT;
