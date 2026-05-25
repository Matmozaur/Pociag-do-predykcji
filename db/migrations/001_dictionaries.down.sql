-- 001_dictionaries.down.sql
BEGIN;

DROP TABLE IF EXISTS disruption_types;
DROP TABLE IF EXISTS train_statuses;
DROP TABLE IF EXISTS stop_types;
DROP TABLE IF EXISTS commercial_categories;
DROP TABLE IF EXISTS stations;
DROP TABLE IF EXISTS carriers;

COMMIT;
