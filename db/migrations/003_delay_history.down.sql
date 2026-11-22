-- 003_delay_history.down.sql
-- Drops the views and the disruption history (the history cannot be rebuilt: PLK only serves the
-- current disruptions).

BEGIN;

DROP TABLE IF EXISTS disruption_history;
DROP VIEW IF EXISTS run_delays;
DROP VIEW IF EXISTS stop_delays;

COMMIT;
