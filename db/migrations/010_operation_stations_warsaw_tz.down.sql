-- 010_operation_stations_warsaw_tz.down.sql
-- Reverse 010_operation_stations_warsaw_tz.up.sql: store the Europe/Warsaw wall clock as UTC again.

BEGIN;

UPDATE operation_stations
SET planned_arrival   = (planned_arrival   AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'UTC',
    planned_departure = (planned_departure AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'UTC',
    actual_arrival    = (actual_arrival    AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'UTC',
    actual_departure  = (actual_departure  AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'UTC';

COMMIT;
