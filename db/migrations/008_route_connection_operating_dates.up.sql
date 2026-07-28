-- 008_route_connection_operating_dates.up.sql
-- Operating dates for individual route connections.

BEGIN;

CREATE TABLE route_connection_operating_dates (
    id                  BIGSERIAL PRIMARY KEY,
    route_connection_id BIGINT NOT NULL REFERENCES route_connections(id) ON DELETE CASCADE,
    operating_date      DATE NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_route_connection_operating_date UNIQUE (route_connection_id, operating_date)
);

CREATE INDEX idx_route_connection_operating_dates_date
    ON route_connection_operating_dates (operating_date);
CREATE INDEX idx_route_connection_operating_dates_connection_id
    ON route_connection_operating_dates (route_connection_id);

COMMIT;
