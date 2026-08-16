-- 009_station_coordinates.down.sql
-- Reverse 009_station_coordinates.up.sql.

BEGIN;

DROP INDEX IF EXISTS idx_stations_coordinates;

ALTER TABLE stations
    DROP COLUMN IF EXISTS longitude,
    DROP COLUMN IF EXISTS latitude;

COMMIT;
