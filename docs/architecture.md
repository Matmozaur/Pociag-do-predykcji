# Architecture

```
PLK Open Data API
   │  Airflow DAGs → pociag_processing (plk → transform → repository)
   ▼
PostgreSQL (db/migrations)
   │  api (Go): handler → service → repository
   ▼
frontend (Next.js), which proxies /bff/* to the api
```

Airflow is the only writer; the api only reads.

## Components

| Component | Responsibility | Contract |
|---|---|---|
| `airflow/plugins/pociag_processing` | Fetch PLK, normalise, upsert. `plk.py` (HTTP), `transform.py` (pure payload → rows), `repository.py` (all SQL), `pipelines.py` (entrypoints) | [specs/pipelines.md](../specs/pipelines.md) |
| `db/migrations` | The schema below | `001_init.up.sql` |
| `services/go/api` | Builds frontend views: `handler` (HTTP) → `service` (views, pure) → `repository` (pgx SQL); `position` is the train-position model | [specs/openapi/api.yml](../specs/openapi/api.yml) |
| `services/frontend` | UI; talks only to the api | `src/lib/api.ts` mirrors `api.yml` |

## Data model

```
trains ◄── schedule_stops ──► stations
trains ◄── operations ◄── operation_stops ──► stations
disruptions ──► stations (start, end)
```

| Table | Row | Key |
|---|---|---|
| `stations` | PLK station, city, coordinates | PLK id |
| `trains` | PLK route (`schedule_id`, `order_id`): name, number, category, carrier, operating dates | surrogate id |
| `schedule_stops` | Planned stop: offsets from local midnight, platform, track | (train, seq) |
| `operations` | A train's run on an operating date and its status (S/P/C/X/Q) | surrogate id; unique (train, date) |
| `operation_stops` | Planned vs actual arrival/departure, delays, confirmed/cancelled | (operation, seq) |
| `disruptions` | Current disruption snapshot | PLK id within the snapshot |

`operations.updated_at` is the last PLK snapshot containing the run; the api uses it to tell
live trains from stale ones.

## Design decisions

- **Airflow is the only ingestion path.** It calls PLK directly; there is no separate collector
  service or raw data lake. Airflow keeps run history; re-running a DAG re-fetches from PLK.
- **One bulk request per timetable day** (`/schedules?dateFrom=D&dateTo=D`) instead of one per
  train.
- **One read service.** The api serves frontend-shaped responses straight from SQL; there is no
  intermediate domain API.
- **Raw PLK quirks are fixed once, at ingestion** (see the rules in `specs/pipelines.md`), so
  readers never compensate for them.
- **Position estimation is pure Go** (`internal/position`): effective times, the active
  predicate, phase and linear interpolation between stops with coordinates.

## Cross-cutting

- Secrets only via env vars / Airflow connections (`infra/.env`, never committed).
- Parameterised SQL only.
- Tracing: OTLP gRPC to the OTel Collector; service names `pociag.api`, `pociag.airflow`.
- Logs: JSON (zap in Go, `logging` in Airflow).
