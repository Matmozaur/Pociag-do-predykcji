-- 011_operation_stations_station_time.down.sql
-- Reverse 011_operation_stations_station_time.up.sql.

BEGIN;

DROP INDEX IF EXISTS idx_operation_stations_station_planned_time;

COMMIT;
