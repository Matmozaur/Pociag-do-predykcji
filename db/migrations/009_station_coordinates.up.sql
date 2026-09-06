-- 009_station_coordinates.up.sql
-- Geographic coordinates for stations, used to render them on the network map.

BEGIN;

ALTER TABLE stations
    ADD COLUMN IF NOT EXISTS latitude  DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION;

-- Speeds up "stations that can be placed on the map" lookups.
CREATE INDEX IF NOT EXISTS idx_stations_coordinates
    ON stations (latitude, longitude)
    WHERE latitude IS NOT NULL AND longitude IS NOT NULL;

COMMIT;
