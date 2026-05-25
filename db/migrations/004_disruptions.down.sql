-- 004_disruptions.down.sql
BEGIN;

DROP TABLE IF EXISTS disruption_affected_routes;
DROP TABLE IF EXISTS disruptions;

COMMIT;
