-- 003_operations.down.sql
BEGIN;

DROP TABLE IF EXISTS operation_stations;
DROP TABLE IF EXISTS train_operations;

COMMIT;
