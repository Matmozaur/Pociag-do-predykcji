-- 002_disruption_id_bigint.up.sql
-- PLK disruption ids are int64 (specs/openapi/plk-open-data.json, DisruptionDto.disruptionId)
-- and now exceed the INTEGER range.

BEGIN;

ALTER TABLE disruptions ALTER COLUMN id TYPE BIGINT;

COMMIT;
