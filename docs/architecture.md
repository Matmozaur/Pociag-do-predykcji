# Architecture

```
PLK Open Data API (X-API-Key)
   │  Airflow 2.10 DAGs → pociag_processing (plk → transform → repository)
   ▼
PostgreSQL 16 "pociag" (db/migrations)
   │  api (Go): handler → service → repository, position model
   ▼
frontend (Next.js server): serves the bundle, rewrites /bff/* → api
   ▲
browser ──► *.tiles.openrailwaymap.org (railway map tiles, third-party)

api, Airflow tasks ──OTLP gRPC──► OTel Collector ──► Jaeger
```

Airflow is the only writer; the api only reads (by convention: it connects as the same `pociag`
role). Everything runs locally under Docker Compose; there is no deployment target yet.

## Components

| Component | Responsibility | Contract |
|---|---|---|
| `airflow/dags` | Three thin TaskFlow DAGs; each task calls one `pociag_processing.pipelines.sync_*` | [specs/pipelines.md](../specs/pipelines.md) |
| `airflow/plugins/pociag_processing` | Fetch PLK, normalise, write. `plk.py` (HTTP), `transform.py` (pure payload → `models.py` rows; all PLK quirks), `repository.py` (all SQL), `pipelines.py` (entrypoints), `tracing.py`, `data/station_coordinates.json` | [specs/pipelines.md](../specs/pipelines.md) |
| `db/migrations` | The schema below (golang-migrate) | `001_init.up.sql` |
| `services/go/api` | Frontend-shaped read API: `handler` (HTTP) → `service` (builds views, pure) → `repository` (pgx SQL); `position` is the train-position model | [specs/openapi/api.yml](../specs/openapi/api.yml) |
| `services/frontend` | Next.js 15 UI, rendered in the browser; all data from the api | `src/lib/api.ts` (hand-written types with `api.yml` field names) |
| `infra/` | Compose stack, OTel Collector, Prometheus, Grafana | `infra/docker-compose.yml` |
| `research/` | Exploratory notebooks (marimo); not part of the runtime | — |

## Ingestion (Airflow)

| DAG | Cron (Europe/Warsaw) | Tasks | Retries / timeout |
|---|---|---|---|
| `sync_schedules` | `30 2 * * *` | `stations` >> `schedules` (today … today+7) | 2 × 10 min / none |
| `sync_operations` | `*/10 * * * *` | `operations` | 0 / 9 min |
| `sync_disruptions` | `*/15 * * * *` | `disruptions` (today, tomorrow) | 1 × 2 min / 14 min |

All DAGs: `catchup=False`, `max_active_runs=1`, start unpaused. Requests and written tables per
DAG are in `specs/pipelines.md`.

- **PLK access** — `PlkClient` (httpx, 180 s timeout, no client retries; Airflow retries apply).
  Connection `pociag_plk`: host = base URL, password = key, sent as `X-API-Key`. Stations and
  operations are paged at the PLK maximum (10 000 / 5 000 per page); cities, schedules (one
  request per day) and disruptions are single requests. Steady-state budget ≈ 144 × operations
  pages + 96 + ~10 requests/day (plus re-fetches on task retries), which must fit the key's tier
  (100–2 000 req/h, 1 000–20 000/day per the upstream spec).
- **Writes** — connection `pociag_postgres` (`PostgresHook`); one transaction per sync, except
  `sync_schedules` (one per day). Stations, coordinates and operation stops skip unchanged rows;
  `trains` (by `sync_schedules`) and `operations` (by `sync_operations`) are always updated, so
  their `updated_at` marks freshness; `schedule_stops` are replaced per train; `disruptions` are
  deleted and re-inserted. `operation_stops` are only upserted: stops PLK drops or renumbers
  for a run are never removed.
- **Referential integrity** — each batch upserts its payload's station dictionary first;
  unknown station ids get placeholder `Stacja <id>` rows and operations for unknown trains get
  bare `trains` rows, so FKs always hold.
- **Station enrichment** — city from `/dictionaries/cities` (only for PLK multi-station cities).
  Coordinates do not come from PLK: the packaged `data/station_coordinates.json` (≈3 000
  stations; curated entries plus OSM matches, © OpenStreetMap contributors, ODbL) is applied by
  the daily `stations` task. `scripts/build_station_coordinates.py` regenerates it offline from
  an Overpass name match; existing entries always win.
- **Time** — naive PLK timestamps are Europe/Warsaw wall-clock; planned operation times that miss
  the overnight day rollover are shifted to within 12 h of the actual time. `operation_stops` hold instants
  (TIMESTAMPTZ); `schedule_stops` hold offsets (INTERVAL) from local midnight of the operating
  date.
- **Packaging** — `pociag_processing` is an installable package (`pociag-processing`) for tests;
  in the stack `airflow/dags` and `airflow/plugins` are bind-mounted read-only and imported from
  the plugins path, relying on the stock `apache/airflow:2.10.4` image for httpx, psycopg2 and
  OTel. Restart `airflow-scheduler` after plugin changes.

## Data model

```
trains ◄── schedule_stops ──► stations
trains ◄── operations ◄── operation_stops ──► stations
disruptions ──► stations (start, end; nullable)
```

| Table | Row | Key |
|---|---|---|
| `stations` | PLK station, city, coordinates | PLK id |
| `trains` | PLK route (`schedule_id`, `order_id`): name, number, category, carrier, `operating_dates` (union of every date seen; empty for bare rows) | surrogate id; unique (`schedule_id`, `order_id`) |
| `schedule_stops` | Planned stop: arrival/departure offsets (may exceed 24 h), platform, track | (train, seq); seq renumbered 1..n |
| `operations` | A train's run on an operating date and its status | surrogate id; unique (train, date) |
| `operation_stops` | Planned vs actual arrival/departure (actual = PLK forecast until `is_confirmed`), delays in minutes, cancelled | (operation, seq); seq = PLK `actualSequenceNumber` |
| `disruptions` | Disruptions for today and tomorrow; dates = min/max affected operating date; `affected_trains` = number of distinct affected runs (the runs themselves are not stored) | PLK id, unique only within one response |

Child rows cascade on delete of their train / operation.

- **Status** — `S` not started, `P` in progress, `C` completed, `X` cancelled, `Q` partly
  cancelled (api: `not_started` … `partial_cancelled`). Other PLK statuses are skipped.
- **`operations.updated_at`** — `now()` (start of the write transaction) of the last
  `sync_operations` run whose PLK snapshot contained the run; one value per sync. The api treats `MAX(updated_at)` as the
  latest snapshot (see Serving).
- **Indexes** — `pg_trgm` GIN on `stations.name` (station search), GIN on
  `trains.operating_dates` (schedule search by date), `schedule_stops(station_id)`,
  `operations(operating_date, status)`, `operations(updated_at)`,
  `operation_stops(station_id, COALESCE(planned_departure, planned_arrival))` (station board).
- **Retention** — none. Operations accumulate as history (the latest PLK state per run); only
  `disruptions` and per-train `schedule_stops` are ever deleted.

## Serving (api)

| Route | Reads |
|---|---|
| `GET /healthz` | — (process alive) |
| `GET /readyz` | DB ping; 503 `not_ready` otherwise |
| `/api/v1/search/stations`, `/api/v1/map/stations` | `stations` |
| `/api/v1/stations/{id}/board` | `stations`, `operation_stops`, `operations`, `trains`, `schedule_stops` |
| `/api/v1/map/trains`, `/api/v1/trains/live`, `/api/v1/trains/{operationId}` | `operations`, `operation_stops`, `trains`, `stations` |
| `/api/v1/schedules/search`, `/api/v1/schedules/{routeId}` | `trains`, `schedule_stops`, `stations` |
| `/api/v1/disruptions` | `disruptions`, `stations` (whole snapshot, no date filter; paginated, most affected first; `severity` from `affected_trains`: ≥ 10 high, ≥ 3 medium) |
| `/api/v1/dashboard/overview` | `operations`, `operation_stops`, `trains`, `disruptions` |

- **Live trains** — a run is live when its status is `P` (or `S` once now ≥ its effective first
  departure), its operating date is today or yesterday (Europe/Warsaw), its `updated_at` is
  within 15 min of the latest snapshot, and now lies in [effective first departure − 5 min,
  effective last arrival], with cancelled stops ignored. SQL pre-filters, `position.IsActive`
  decides; carrier filtering and `/trains/live` pagination happen in Go. Freshness is relative to
  the latest snapshot, so if ingestion stalls the last snapshot's trains stay live and only their
  `confidence` drops.
- **Position model** (`internal/position`, pure Go) — effective times (actual, else planned +
  delay), the active predicate, phase, a position (`station`, or linear interpolation between the
  nearest stops with coordinates: `interpolated` / `interpolated_sparse`; runs without anchors
  are counted as `unpositioned_count`), last confirmed delay and confidence (`high`: the run
  was seen < 15 min ago and its last confirmed stop departed < 20 min ago; `medium`: seen
  < 30 min ago; else `low`).
- **Station board** — stops planned in [now − 6 h, now + 12 h], excluding `X` runs; bucketed into
  at station (terminus: 5 min after arrival), arrivals and departures.
- **Dashboard** — today's Warsaw operating date: counts by status (`in_progress` uses the 15 min
  snapshot window), on-time ≤ 5 min by maximum confirmed delay, and data freshness
  (`MAX(trains.updated_at)` over trains with operating dates, `MAX(operations.updated_at)`,
  disruption count).
- **Time** — operating dates and "today" are Europe/Warsaw; times shown to people are Warsaw
  `HH:MM`, built in `service`; instants are RFC 3339. `time/tzdata` is embedded (the image has no
  zoneinfo).
- **HTTP** — chi; open CORS (`*`, `GET, OPTIONS`); bad parameters → 400 `invalid_request`,
  missing rows → 404 `not_found`, else 500 `internal_error` logged with `trace_id`; errors are
  `{error, message}`. `ReadHeaderTimeout` 5 s, `WriteTimeout` 30 s; graceful shutdown on
  SIGINT/SIGTERM (10 s). The pgx pool connects lazily, so the api starts without a reachable DB.
- **Config** — `HTTP_ADDR`, `DATABASE_DSN` (required); `OTEL_EXPORTER_OTLP_ENDPOINT` (optional,
  `host:port`; tracing is off when unset).

## Frontend

- **Rendering** — every route page is a client component (the root layout and the `/` →
  `/mapa` server `redirect()` are the only server components); data is fetched in the browser
  with TanStack Query (default `staleTime` 30 s, `retry` 1). The Next server only serves the
  bundle, redirects and rewrites `/bff/*`; there is no SSR data fetching or server cache.
- **Request path** — browser → `/bff/*` on the frontend origin → Next rewrite → `API_URL`
  (default `http://localhost:8080`). Next fixes rewrites at build time (`routes-manifest.json`):
  the Dockerfile builds with `http://api:8080`, so the runtime `API_URL` in compose has no
  effect and changing the api host means rebuilding the image. The api never needs to be reachable from the browser.
  `NEXT_PUBLIC_API_BASE` optionally overrides the `/bff` prefix at build time.

| Route | Endpoints | Refresh |
|---|---|---|
| `/mapa` | `map/trains`, `map/stations`, `dashboard/overview`, `trains/live?limit=4`, `disruptions?limit=3`, board of the selected station | trains poll 30 s; live list and board 60 s; stations stale 1 h; overview / disruptions stale 30 / 60 s |
| `/pociagi`, `/pociagi/[id]` | `trains/live?limit=50`, `dashboard/overview`; `trains/{id}` | poll 60 s |
| `/rozklad/[id]` | `schedules/{id}` | stale 5 min |
| `/utrudnienia` | `disruptions` (top 20) | stale 2 min |
| `/wyszukaj` | `search/stations` (debounced 300 ms), `schedules/search` | on input (stale 60 s) / submit |
| `/stations/[id]` | `stations/{id}/board` | poll 60 s |

- **Map** — react-leaflet, no base map: Poland fill from `public/poland-border.geojson`,
  OpenRailwayMap track tiles (CC-BY-SA), an outside-Poland mask, then stations and trains on a
  canvas. Train positions are re-interpolated in the browser every 5 s between polls.
- **Freshness** — the UI flags data older than 20 min (`operations_last_updated` / `data_as_of`).

## Runtime (Docker Compose)

`infra/docker-compose.yml`, project `pociag`, configured from `infra/.env` (copy of
`infra/.env.example`).

| Service | Image | Profile | Host → container | Starts after | Role |
|---|---|---|---|---|---|
| `postgres` | postgres:16-alpine | — (always) | 5434 → 5432 | — (healthcheck) | Curated DB `pociag` |
| `migrate` | migrate/migrate:v4.18.1 | — (always) | — | postgres healthy | `up` on `db/migrations`, exits |
| `otel-collector` | otel/opentelemetry-collector-contrib:0.117.0 | — (always) | 4327 → 4317, 4328 → 4318, 8889 | — | OTLP receiver |
| `api` | build `services/go/api` | — (always) | 8080 → 8080 | migrate done, collector | Read API |
| `frontend` | build `services/frontend` | frontend | `${FRONTEND_PORT:-3100}` → 3000 | api started | Next.js |
| `airflow-db` | postgres:16-alpine | airflow | 5433 → 5432 | — (healthcheck) | Airflow metadata only |
| `airflow-init` | apache/airflow:2.10.4 | airflow | — | airflow-db healthy | `db migrate` + `admin` user, exits |
| `airflow-webserver` | apache/airflow:2.10.4 | airflow | 8090 → 8080 | airflow-init done | UI / REST API |
| `airflow-scheduler` | apache/airflow:2.10.4 | airflow | — | airflow-init, migrate done | Runs tasks (LocalExecutor) |
| `jaeger` | jaegertracing/all-in-one:1.63.0 | tracing | 16686 | — | Trace UI, in-memory |
| `prometheus` | prom/prometheus:v3.1.0 | monitoring | 9090 | — | Scrapes collector :8889, 15 d retention |
| `grafana` | grafana/grafana:11.4.0 | monitoring | 3001 → 3000 | prometheus | Anonymous viewer; nothing provisioned |

- **Profiles** — services without a profile start with every invocation. `make up` =
  `--profile all` (everything); `make up-core` = no profile: postgres, migrate, otel-collector,
  api.
- **Networks** — every service is on `default` except the frontend, which is only on `web`; the
  api is on both, so the frontend can reach only the api.
- **Volumes** — `postgres_data`, `airflow_db_data`, `airflow_logs`, `prometheus_data`,
  `grafana_data`. Only the two Postgres services have healthchecks.
- **Migrations** — the `migrate` service applies them on every `up`; `make db-migrate-*` runs the
  same image on the host network against `DB_URL` (port 5434).
- **Config** — `PLK_BASE_URL`, `PLK_API_KEY` (required), `POSTGRES_PASSWORD`,
  `AIRFLOW_DB_PASSWORD`, `AIRFLOW__CORE__FERNET_KEY`, `AIRFLOW__WEBSERVER__SECRET_KEY`; optional
  `FRONTEND_PORT`. Airflow connections are `AIRFLOW_CONN_*` env vars built from these, not rows in
  the Airflow DB.
- **Images** — api: `golang:1.25.0-alpine3.22` → `alpine:3.22.1`, static binary, non-root (uid
  1001). frontend: `node:22-alpine` deps → build → `next start` on 3000 (runs as root).
  Everything else is a stock image.

## Observability

```
api, Airflow tasks ──OTLP gRPC (insecure)──► otel-collector:4317
  traces  → memory_limiter, batch, resource (deployment.environment=local) → Jaeger + debug
  metrics → Prometheus exporter :8889 (namespace pociag) ◄── Prometheus (15 s scrape) ◄── Grafana
  logs    → debug
```

- **Traces** — api: `otelhttp` server spans (all named `pociag.api`) with repository child spans
  `db.<noun>.<verb>` (mostly table names), W3C TraceContext. Airflow: `<sync>.sync` › `plk.get` /
  `db.<table>.save|replace`. Both export only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set (compose
  sets it). Jaeger runs only with the `tracing` profile, but the
  collector (always on) always exports to it, so without that profile it logs export errors.
- **Metrics** — the pipeline exists, but no component emits metrics and Grafana has no datasource
  or dashboards, so Prometheus/Grafana show no application data yet.
- **Logs** — container stdout via Docker `json-file` (10 MB × 3; the one-shot `migrate` and
  `airflow-init` use the daemon default). api: zap JSON (startup,
  shutdown, readiness, 500s; no access log). Airflow: stdlib `logging` in Airflow's default text
  format; task logs in the `airflow_logs` volume. No logs are shipped to the collector.

## CI

`.github/workflows/ci-cd.yaml` on every push and PR: `api` (`go test -race`, golangci-lint v2, unpinned `@latest`)
and `airflow` (ruff, mypy strict via `pyproject.toml`, pytest), each with a Postgres 16 service for the SQL tests,
which load `001_init.up.sql` directly. Not in CI: frontend build, image builds. The build and
deploy jobs are placeholders.

## Design decisions

- **Airflow is the only ingestion path.** It calls PLK directly; there is no separate collector
  service or raw data lake. Airflow keeps run metadata and task logs, not payloads; re-running a
  DAG re-fetches from PLK.
- **One bulk request per timetable day** (`/schedules?dateFrom=D&dateTo=D`) instead of one per
  train.
- **One read service.** The api builds frontend-shaped responses in `service` from SQL rows;
  there is no intermediate domain API.
- **Raw PLK quirks are fixed once, at ingestion** (see the rules in `specs/pipelines.md`), so
  readers never compensate for them.
- **Position estimation is pure Go** (`internal/position`), testable without a database.
- **The browser talks only to the frontend origin** (plus map tiles); the frontend reaches the
  api over the internal `web` network. The api is still published on host :8080 for development.

## Cross-cutting

- Secrets via env vars (`infra/.env`, git-ignored) and Airflow `AIRFLOW_CONN_*` env vars. The
  committed defaults — `.env.example` passwords and Fernet key, the Airflow `admin/admin` user,
  the Makefile `DB_URL` — are for local development only.
- No authentication on the api or the frontend (read-only public data); the Airflow UI and REST
  API use Airflow login / basic auth.
- Parameterised SQL only.
- Tracing service names `pociag.api`, `pociag.airflow`; span names `<noun>.<verb>`.
- Never log secrets or payloads.
