-- 002_schedules.down.sql
BEGIN;

DROP TABLE IF EXISTS route_connections;
DROP TABLE IF EXISTS route_stations;
DROP TABLE IF EXISTS route_operating_dates;
DROP TABLE IF EXISTS routes;

COMMIT;
