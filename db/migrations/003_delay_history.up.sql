-- 003_delay_history.up.sql
-- Training data for delay models. operations / operation_stops are never purged, so they already
-- hold every run the syncs have seen. This adds:
--   * stop_delays / run_delays: views that label each stop and each run with its delay, and say
--     when the outcome is not known yet (no new storage, always consistent with the tables);
--   * disruption_history: every state a disruption has had (the disruptions table is replaced by
--     each sync, so without this its history is lost).

BEGIN;

-- One row per stop PLK reports as having happened (confirmed, not cancelled), with its delay in
-- minutes: negative is early. PLK leaves the delay minutes out of an on-time stop, so a NULL
-- there is 0. An arrival needs a planned arrival (the origin has none) and an actual arrival; a
-- departure likewise (the terminus has none). Minutes are PLK's, whole minutes, as in the api.
CREATE VIEW stop_delays AS
SELECT o.id AS operation_id,
       o.train_id,
       o.operating_date,
       s.seq,
       s.station_id,
       s.planned_arrival,
       s.actual_arrival,
       s.planned_departure,
       s.actual_departure,
       CASE WHEN s.planned_arrival IS NOT NULL AND s.actual_arrival IS NOT NULL
            THEN COALESCE(s.arrival_delay, 0) END AS arrival_delay_min,
       CASE WHEN s.planned_departure IS NOT NULL AND s.actual_departure IS NOT NULL
            THEN COALESCE(s.departure_delay, 0) END AS departure_delay_min
FROM operation_stops s
JOIN operations o ON o.id = s.operation_id
WHERE s.is_confirmed AND NOT s.is_cancelled;

-- One row per run. The final stop is the last one that is not cancelled and has a planned
-- arrival. final_arrival_delay_min and is_delayed (final arrival more than 5 minutes late, the
-- api's on-time threshold) are NULL while that arrival is unconfirmed: a run PLK stopped reporting
-- before it finished (status S, P or Q on an old operating date) has no known outcome.
-- Cancelled runs (status X) have no delay either. Use `WHERE is_delayed IS NOT NULL` for labels.
CREATE VIEW run_delays AS
SELECT o.id AS operation_id,
       o.train_id,
       o.operating_date,
       o.status,
       counts.stops,
       counts.confirmed_stops,
       counts.cancelled_stops,
       worst.max_arrival_delay_min,
       final.station_id AS final_station_id,
       final.arrival_delay_min AS final_arrival_delay_min,
       final.arrival_delay_min > 5 AS is_delayed
FROM operations o
CROSS JOIN LATERAL (
    SELECT count(*)::int AS stops,
           count(*) FILTER (WHERE s.is_confirmed)::int AS confirmed_stops,
           count(*) FILTER (WHERE s.is_cancelled)::int AS cancelled_stops
    FROM operation_stops s WHERE s.operation_id = o.id
) counts
CROSS JOIN LATERAL (
    SELECT max(d.arrival_delay_min) AS max_arrival_delay_min
    FROM stop_delays d WHERE d.operation_id = o.id
) worst
LEFT JOIN LATERAL (
    SELECT s.station_id, d.arrival_delay_min
    FROM operation_stops s
    LEFT JOIN stop_delays d ON d.operation_id = s.operation_id AND d.seq = s.seq
    WHERE s.operation_id = o.id AND s.planned_arrival IS NOT NULL AND NOT s.is_cancelled
    ORDER BY s.seq DESC
    LIMIT 1
) final ON o.status <> 'X';

-- Disruptions: one row per distinct state (what the sync saw), so a disruption whose
-- affected_trains or dates change has several rows. first_seen_at / last_seen_at bound the syncs
-- that saw exactly that state. PLK ids are only unique within one response, so none is kept.
CREATE TABLE disruption_history (
    id               BIGSERIAL PRIMARY KEY,
    type_code        TEXT,
    start_station_id INTEGER REFERENCES stations (id),
    end_station_id   INTEGER REFERENCES stations (id),
    message          TEXT NOT NULL,
    date_from        DATE,
    date_to          DATE,
    affected_trains  INTEGER NOT NULL,
    first_seen_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (type_code, start_station_id, end_station_id, message, date_from, date_to, affected_trains)
);

CREATE INDEX disruption_history_first_seen_idx ON disruption_history (first_seen_at);
CREATE INDEX disruption_history_start_station_idx ON disruption_history (start_station_id);
CREATE INDEX disruption_history_end_station_idx ON disruption_history (end_station_id);

COMMIT;
