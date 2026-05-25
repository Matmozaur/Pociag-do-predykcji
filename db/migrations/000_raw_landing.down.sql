-- 000_raw_landing.down.sql
BEGIN;

DROP TABLE IF EXISTS raw_dictionaries;
DROP TABLE IF EXISTS raw_disruptions;
DROP TABLE IF EXISTS raw_operations;
DROP TABLE IF EXISTS raw_schedules;
DROP EXTENSION IF EXISTS pg_trgm;

COMMIT;
