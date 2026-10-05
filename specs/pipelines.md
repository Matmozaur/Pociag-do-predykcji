# Pipelines

Airflow DAGs in `airflow/dags/`, logic in `airflow/plugins/pociag_processing/`. Every sync is
idempotent and writes one PLK response (or one day) per transaction. Schedules are cron in
Europe/Warsaw; `catchup=False`, `max_active_runs=1`.

| DAG | Schedule | PLK request(s) | Writes |
|---|---|---|---|
| `sync_schedules` | `30 2 * * *` | task `stations`: `/dictionaries/stations`, `/dictionaries/cities`; task `schedules`: `/schedules?dateFrom=D&dateTo=D` for today … today+7 | `stations` (+ city, packaged coordinates); `trains` upsert (operating dates merged), `schedule_stops` replaced per train |
| `sync_operations` | `*/10 * * * *` | `/operations?withPlanned=true`, all pages | `operations` upsert (`updated_at` = seen in this snapshot), `operation_stops` upsert (unchanged rows skipped) |
| `sync_disruptions` | `*/15 * * * *` | `/disruptions?dateFrom=today&dateTo=today+1` | `disruptions` replaced |

## PLK data rules (`transform.py`)

- Naive PLK timestamps are Europe/Warsaw wall-clock time.
- Schedule times are `HH:MM:SS` + day offset → `INTERVAL` from local midnight of the operating
  date (can exceed 24 h or be negative). PLK order numbers are sparse; stops are renumbered 1..n.
- Planned operation times after midnight on overnight trains lack the day rollover; they are
  shifted by whole days to within 12 h of the actual time.
- Each payload carries a dictionary of the stations it references; it is upserted first. Station
  ids missing from every dictionary get a placeholder `Stacja <id>` until a dictionary names them.
- An operation for a train not in the timetable creates a bare `trains` row.
- Disruption ids are only unique within one response, hence snapshot replacement. A disruption's
  dates are the min/max operating date of its affected trains; a message that is only a type
  code is replaced by that type's description.

## Connections

`pociag_postgres` (curated DB) and `pociag_plk` (host = PLK base URL, password = API key),
defined as `AIRFLOW_CONN_*` env vars in `infra/docker-compose.yml`.
