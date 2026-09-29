-- 010_operation_stations_warsaw_tz.up.sql
-- One-shot data correction: PLK operation stop times are naive Europe/Warsaw wall-clock values
-- but were stored as if they were UTC (1-2 h late). Re-interpret the stored wall clock as
-- Europe/Warsaw so the columns hold correct UTC instants.
--
-- Must run exactly once, together with the pociag_processing release that parses these fields
-- as Europe/Warsaw. Pause the operations DAGs, deploy the plugin, apply this migration, unpause.

BEGIN;

UPDATE operation_stations
SET planned_arrival   = (planned_arrival   AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw',
    planned_departure = (planned_departure AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw',
    actual_arrival    = (actual_arrival    AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw',
    actual_departure  = (actual_departure  AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw';

COMMIT;
