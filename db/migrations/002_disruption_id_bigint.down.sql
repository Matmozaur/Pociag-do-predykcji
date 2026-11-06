-- 002_disruption_id_bigint.down.sql
-- Disruptions are a snapshot replaced every sync, so rows that no longer fit are dropped.

BEGIN;

DELETE FROM disruptions WHERE id NOT BETWEEN -2147483648 AND 2147483647;
ALTER TABLE disruptions ALTER COLUMN id TYPE INTEGER;

COMMIT;
