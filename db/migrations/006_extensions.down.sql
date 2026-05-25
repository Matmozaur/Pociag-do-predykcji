-- 006_extensions.down.sql
BEGIN;

DROP EXTENSION IF EXISTS pg_trgm;

COMMIT;
