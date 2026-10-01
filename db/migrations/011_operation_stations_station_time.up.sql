-- 011_operation_stations_station_time.up.sql
-- Station board lookups: stops at one station within a planned-time range. The expression must
-- match the station board query predicate in data-service exactly.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_operation_stations_station_planned_time
    ON operation_stations (station_external_id, (COALESCE(planned_departure, planned_arrival)));

COMMIT;
