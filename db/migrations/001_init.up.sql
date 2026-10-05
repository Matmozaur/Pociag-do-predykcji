-- 001_init.up.sql
-- Curated schema. Written by the Airflow pociag_processing plugin, read by the api service.
-- All PLK wall-clock times are Europe/Warsaw; TIMESTAMPTZ columns hold the resulting instants.

BEGIN;

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Railway stations: PLK station dictionary, enriched with city and curated coordinates.
CREATE TABLE stations (
    id         INTEGER PRIMARY KEY,                -- PLK station id
    name       TEXT NOT NULL,
    city       TEXT,                               -- only for PLK multi-station cities
    latitude   DOUBLE PRECISION,
    longitude  DOUBLE PRECISION,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX stations_name_trgm_idx ON stations USING gin (name gin_trgm_ops);

-- Trains: one PLK route (schedule_id, order_id) and its timetable parameters.
-- Operations of a train that is not (yet) in the schedules create a bare row.
CREATE TABLE trains (
    id              BIGSERIAL PRIMARY KEY,
    schedule_id     INTEGER NOT NULL,
    order_id        INTEGER NOT NULL,
    name            TEXT,                          -- marketing or line name, e.g. "S1"
    number          TEXT,                          -- national train number
    category        TEXT,                          -- commercial category symbol, e.g. IC, TLK, R
    carrier_code    TEXT,
    carrier_name    TEXT,
    operating_dates DATE[] NOT NULL DEFAULT '{}',  -- every date seen in a schedules sync
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (schedule_id, order_id)
);

CREATE INDEX trains_operating_dates_idx ON trains USING gin (operating_dates);

-- Schedules: the planned stops of a train. arrival/departure are offsets from local midnight of
-- the operating date (PLK day offset + wall-clock time), so they can exceed 24h.
CREATE TABLE schedule_stops (
    train_id   BIGINT NOT NULL REFERENCES trains (id) ON DELETE CASCADE,
    seq        SMALLINT NOT NULL,                  -- 1-based position along the route
    station_id INTEGER NOT NULL REFERENCES stations (id),
    arrival    INTERVAL,
    departure  INTERVAL,
    platform   TEXT,
    track      TEXT,
    PRIMARY KEY (train_id, seq)
);

CREATE INDEX schedule_stops_station_idx ON schedule_stops (station_id);

-- Operations: a train's run on one operating date, as last reported by PLK.
CREATE TABLE operations (
    id             BIGSERIAL PRIMARY KEY,
    train_id       BIGINT NOT NULL REFERENCES trains (id) ON DELETE CASCADE,
    operating_date DATE NOT NULL,
    status         CHAR(1) NOT NULL CHECK (status IN ('S', 'P', 'C', 'X', 'Q')),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),  -- last PLK snapshot containing the run
    UNIQUE (train_id, operating_date)
);

CREATE INDEX operations_date_status_idx ON operations (operating_date, status);
CREATE INDEX operations_updated_at_idx ON operations (updated_at);

-- Operation events: planned vs actual arrival and departure of a run at each stop.
-- actual_* is PLK's forecast until the stop is confirmed.
CREATE TABLE operation_stops (
    operation_id      BIGINT NOT NULL REFERENCES operations (id) ON DELETE CASCADE,
    seq               SMALLINT NOT NULL,           -- PLK actual sequence number
    station_id        INTEGER NOT NULL REFERENCES stations (id),
    planned_arrival   TIMESTAMPTZ,
    planned_departure TIMESTAMPTZ,
    actual_arrival    TIMESTAMPTZ,
    actual_departure  TIMESTAMPTZ,
    arrival_delay     INTEGER,                     -- minutes, as reported by PLK
    departure_delay   INTEGER,
    is_confirmed      BOOLEAN NOT NULL DEFAULT false,
    is_cancelled      BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (operation_id, seq)
);

-- Station board: stops at one station within a planned-time window.
CREATE INDEX operation_stops_station_time_idx
    ON operation_stops (station_id, (COALESCE(planned_departure, planned_arrival)));

-- Disruptions: the current PLK disruption snapshot, replaced on every sync. PLK ids are only
-- unique within one response, so rows are never updated in place.
CREATE TABLE disruptions (
    id               INTEGER PRIMARY KEY,          -- PLK id within the snapshot
    type_code        TEXT,                         -- e.g. utr_32 (replacement bus)
    start_station_id INTEGER REFERENCES stations (id),
    end_station_id   INTEGER REFERENCES stations (id),
    message          TEXT NOT NULL,
    date_from        DATE,                         -- first / last operating date of the
    date_to          DATE,                         -- affected trains
    affected_trains  INTEGER NOT NULL DEFAULT 0,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
