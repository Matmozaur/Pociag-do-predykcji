-- 001_init.down.sql

BEGIN;

DROP TABLE IF EXISTS disruptions;
DROP TABLE IF EXISTS operation_stops;
DROP TABLE IF EXISTS operations;
DROP TABLE IF EXISTS schedule_stops;
DROP TABLE IF EXISTS trains;
DROP TABLE IF EXISTS stations;
DROP EXTENSION IF EXISTS pg_trgm;

COMMIT;
