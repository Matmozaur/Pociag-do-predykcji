# Proposal: Train positions on the map

**Status:** proposal, not yet implemented. Branch `feature/train-positions`.
**Request:** "Show all trains on the map with their positions, even if approximate. This may need DB and Airflow pipeline changes."
**Related work:** `fix/data-freshness` (why routes that have ended still show as current), `feature/station-board`,
`feature/train-details`. Both of the latter should reuse the **Active train model** defined in §4.

---

## 1. TL;DR

- PLK does **not** publish train GPS positions. It publishes a real-time *operations snapshot* (`GET /api/v1/operations`).
  For each stop the snapshot gives the planned time and an "actual" time. For stops a train has already passed, the
  actual time is observed. For stops still ahead, it is **PLK's own forecast**. We can estimate a position by
  **interpolating between the previous and the next stop, using these effective times and the current time**.
- **Recommendation:** compute the position **when a client asks for it, in data-service (Go)**, using one bulk SQL
  query plus a pure, unit-tested estimator. Do not precompute a positions table in Airflow. Positions move with the
  clock between snapshots, so a stored position would be stale as soon as it was written. The only thing that must
  change in ingestion is **how often it runs**: add an `ingest_operations_live` DAG every 10 min, next to the daily
  one.
- **Three data problems block this feature and must be fixed first:**
  1. **Operation timestamps are stored 2 h off (CEST).** PLK sends naive local times (`"2026-09-28T10:30:00"`). The
     plugin stores them as UTC (§3.3). Interpolating against `now()` would put every train 1–2 h off.
  2. **Only 93 of 3,319 stations have coordinates.** Only 91 of the 2,497 stations used by today's in-progress
     trains are covered (§3.4). Without more coordinates most trains have nothing to interpolate between.
  3. **Operations are ingested once a day** (`0 2 * * *`), so each snapshot is up to 24 h old.
- **New endpoints (contract-first):** data-service `GET /api/v1/operations/active` and gateway `GET /api/v1/map/trains`.
  The new DB migration is a data backfill for the timezone fix, not a new table.

---

## 2. Current state (evidence)

### 2.1 What the map shows today
| What | Evidence |
|---|---|
| Station markers from `GET /bff/api/v1/map/stations`, 1 h stale time | `services/frontend/src/components/TrafficMapClient.tsx:84-88`, `:145-170` |
| Railway tracks are **raster tiles** from OpenRailwayMap. The system has no vector track geometry | `TrafficMapClient.tsx:61-67`, `:124-132` |
| The side panel shows 4 "live" trains as text only (no position), refreshed every 60 s | `services/frontend/src/components/MapHomeClient.tsx:80` |
| Gateway `/api/v1/map/stations` keeps only stations that have lat/lon | `services/go/gateway/internal/service/service.go:85-111` |
| Gateway `/api/v1/trains/live` builds `current_station`/`next_station` from the last `is_confirmed` stop. It makes an **N+1** call to data-service for each operation and uses the **UTC** date as "today" | `gateway/internal/service/service.go:322-372` (N+1 at `:340`, UTC date at `:326`), helper `:811-858` |
| The spec already says `LiveTrainSummary` carries "current position", but there are no coordinates | `specs/openapi/gateway.yml:204-236`, `:602-640` |

### 2.2 Data we have
| Table | Relevant columns | Evidence |
|---|---|---|
| `stations` | `external_id`, `name`, `latitude`, `longitude` (nullable) | `db/migrations/001_dictionaries.up.sql:22-29`, `009_station_coordinates.up.sql:6-8` |
| `train_operations` | `(schedule_id, order_id, operating_date)` unique, `train_status` S/P/C/X/Q, `updated_at` (set on every upsert, so it records when the train was last seen in a snapshot) | `003_operations.up.sql:8-23`, upsert `airflow/plugins/pociag_processing/repository.py:564-648` |
| `operation_stations` | `actual_sequence_number`, `planned_arrival/departure`, `actual_arrival/departure`, delays, `is_confirmed`, `is_cancelled` | `003_operations.up.sql:26-49` |
| `routes` / `route_stations` | name, carrier, planned stop list with `INTERVAL` + day offset | `002_schedules.up.sql:8-69` |

Live DB check on 2026-09-27 (read-only):
- `train_operations`: 163,723 rows. `operation_stations`: 2.87 M rows. Operating dates 2026-07-23 to 2026-09-28.
- The latest snapshot (`updated_at` 06:13Z) holds about **22.5 k trains across 4 operating dates**. In it, 506 trains
  are `P` for today, and **211 and 206 trains are still `P` for the two previous days**. Status alone is not
  enough to decide "active". This overlaps with `fix/data-freshness`.
- On a sample in-progress train, **every future stop has `actual_*` filled** with PLK's delay-propagated
  forecast (`is_confirmed=false`). Across today's rows, 24,423 unconfirmed stops have `actual_*` set, and 0
  confirmed stops lack it. So `actual_*` works as an "effective time" for every stop.
- Only 4 of 11,520 stop rows of today's `P` trains have no planned time at all.
- 2,339 of today's 2,340 operations **have no matching `routes` row**. The local schedules were last ingested in
  July (`route_operating_dates` ends 2026-08-02; the local stack does not run continuously). The position logic must
  **not depend on `routes`**. Train name and carrier are optional enrichment.

### 2.3 What PLK offers (`specs/openapi/plk-open-data.json`)
- `GET /api/v1/operations` returns a real-time snapshot. It has **no date or status filter**, only
  `stations`, `carriersInclude/Exclude`, `page`, `pageSize` (max **5000**), `fullRoutes` and `withPlanned`. The
  response carries `generatedAt`.
- `GET /api/v1/operations/train/{scheduleId}/{orderId}/{operatingDate}` returns one train (useful for train details
  later).
- There are **no coordinates anywhere**: `StationDto` = `{id, name}` only.
- Rate limits: Basic 100 req/h and 1,000 req/day. Standard 500/h and 5,000/day. Premium 2,000/h and 20,000/day.

### 2.4 Ingestion today
| What | Evidence |
|---|---|
| The collector calls `/api/v1/operations?withPlanned=true` with `pageSize=1000`, about 23 pages per snapshot | `services/go/collector/internal/plk/client.go:77-82`, `internal/service/service.go:383-423` |
| The capture date is `time.Now().UTC()`. Objects land at `raw/operations/YYYY/MM/DD/run_<id>_page_<n>.parquet` (JSON envelope) | `collector/internal/service/service.go:186-226`, `internal/lake/store.go:111-135` |
| DAG `ingest_operations_daily` runs `0 2 * * *` (fetch ops → process → fetch disruptions → process). `capture_date = date.today()` (worker TZ) | `airflow/dags/ingest_operations_daily.py:51-62` |
| The spec still says "Pull yesterday's completed operations" | `specs/airflow-dags.md:97-104` |
| The plugin upserts **row by row** (`cur.execute` per stop). A full snapshot took about **2 min** (06:11:30 → 06:13:20) | `repository.py:599-641`, `ingestion_runs` |
| Raw volume is **~125 MB per snapshot** (MinIO `raw/operations/2026/09/27` = 250 MB for 2 runs, 46 objects) | MinIO data dir |
| The pipeline lock is `is_pipeline_running(pipeline, run_date)`. It is keyed per day, and a run that crashes stays `running` forever | `repository.py:63-81` |
| AsyncAPI defines `pociag.data.operations_ingested`, but nothing emits `NOTIFY` yet | `specs/asyncapi/platform-events.yml:54-60`; no `pg_notify` in code |

### 2.5 Data gaps (blocking or significant)
1. **Timezone bug (blocking).** The raw payload has `"plannedDeparture":"2026-09-28T10:30:00"`: no offset,
   Europe/Warsaw wall-clock. `_parse_timestamp` (`repository.py:39-42`) returns a naive datetime. The DB session
   TZ is `UTC`, so the value is stored as 10:30**Z**. Evidence: in the 06:1xZ snapshot, **9,233 of 18,422 confirmed
   stops** have `actual_departure > 06:11Z`, which is impossible. Read as local time, only 13 do. The UI shows the
   right "HH:MM" only by accident: `trainutil.FormatClock` formats the UTC value (`shared/trainutil/util.go:51-57`).
2. **Station coordinates (significant).** 93 of 3,319 stations have coordinates, from the hand-curated
   `airflow/plugins/pociag_processing/data/station_coordinates.json` (applied at `pipelines/dictionaries.py:100-106`).
   Only 91 of the 2,497 distinct stations on today's `P` trains are covered.
3. **Freshness (significant).** One snapshot per day. Every position would be a forecast up to 24 h old.
4. **"Active" is ambiguous.** PLK leaves some trains `P` for days (§2.2). Coordinate with `fix/data-freshness`.
5. **No track geometry.** Straight lines between stations are the best we can do without a new geometry import.

---

## 3. Design options

### 3.1 Where to compute the position
| Option | How | Pros | Cons |
|---|---|---|---|
| **A. On read in data-service (recommended)** | One bulk SQL query loads the active operations with their stops and station coords. A pure Go estimator computes the position at `now` | Positions advance with the clock between snapshots. No new table or writes. Logic is unit-testable and reusable by the station board and train detail. `?at=` allows replay | Some CPU per request: about 1–2 k trains × about 20 stops, which is trivial. Needs a short in-process cache if polled heavily |
| B. Precomputed `train_positions` table written by Airflow | The plugin computes positions after each upsert | Cheapest read | Positions are frozen for 10 min and then jump. Logic ends up in Python *and* Go (siblings need it in Go). Adds write load and a new table in both specs |
| C. Pure SQL view (`LATERAL` last/next stop plus lerp in SQL) | View or function in Postgres | No Go logic | Hard to test. Coordinate-anchor search in SQL is awkward. Spec and view drift |
| D. Bypass the lake (collector writes positions straight to Postgres) | — | Lowest latency | Violates the ELT / read-write separation (ADR-001/003). Rejected |

**Recommendation: A.** Airflow keeps doing what it does (land the snapshot, then upsert curated operations), just
more often.

### 3.2 Ingestion frequency
| Cadence | PLK calls/day (pageSize 5000, ~5 pages) | Fits tier | Raw volume/day (~125 MB/snapshot, uncompressed) | Staleness |
|---|---|---|---|---|
| daily (today) | ~23 | any | 0.1 GB | ≤24 h |
| **every 10 min (recommended start)** | ~720 (+ other DAGs) | Basic, tight: 1,000/day. Standard, comfortable | ~18 GB → **needs retention** | ≤10 min + processing |
| every 5 min | ~1,440 | Standard+ | ~36 GB | ≤5 min |

Recommended: a new **`ingest_operations_live`** DAG, `*/10 * * * *`, `max_active_runs=1`, `catchup=False`,
`dagrun_timeout=9 min`, running only `fetch_operations` → `process_operations`. Make the cadence an Airflow Variable
with default 10 so it can be tightened without a code change. Keep `ingest_operations_daily` for the disruptions
and end-of-day capture. Required alongside it:
- raise the collector's `pageSize` 1000 → 5000 (about 5× fewer requests; within the PLK maximum);
- add a MinIO lifecycle rule that expires `raw/operations/` after N days (devops);
- batch the plugin upsert (`execute_values`), because 2 min per run is too slow for a 10-min cadence;
- add a stale-lock guard: ignore a `running` `ingestion_runs` row older than 30 min, so one crash does not block the
  rest of the day.

### 3.3 Timezone fix (prerequisite)
| Option | Pros | Cons |
|---|---|---|
| **Fix at ingest (recommended).** The plugin treats naive PLK timestamps as `Europe/Warsaw`. A migration backfills existing rows. Go `FormatClock` renders in `Europe/Warsaw` | Stored instants become correct. Every consumer (positions, station board, delay analytics) can compare against `now()` | Touches plugin, migration and shared Go formatting in one coordinated change |
| Work around in the estimator: compare against "Warsaw wall-clock as fake UTC" | No data change | Spreads the bug to every new feature. DST edge cases |

The backfill as migration `010`:
- up: `ts := (ts AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw'`
- down: `ts := (ts AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'UTC'`
- columns: the four timestamp columns of `operation_stations`

**This may belong to `fix/data-freshness`.** Whichever branch lands first owns it. Step 0 below is written so
that either branch can run it.

### 3.4 Station coordinates
| Option | Coverage | Cost |
|---|---|---|
| Hand-curate more entries in `station_coordinates.json` | slow, partial | manual |
| **Offline OSM match (recommended).** A script queries Overpass for `railway=station|halt` in PL, matches by normalised name (plus `city` disambiguation), and regenerates `station_coordinates.json`, with a `source`/ODbL attribution and a manual-override section | ~85–95 % expected; report unmatched | one script, no runtime dependency; follows the existing packaged-JSON pattern |
| Geocode at runtime | high | external dependency in the pipeline, rate limits. Rejected |

The estimator still copes with gaps: it interpolates between the **nearest stops that have coordinates** before
and after the train (`method=interpolated_sparse`). If there are none, it returns `position=null` and the train is
listed but not drawn.

### 3.5 Following the track instead of straight lines
Straight lines between stations are good enough for "approximate" at country zoom. A later phase could import OSM
railway ways, precompute a polyline per consecutive station pair, and store it in a `station_segments` table. That
needs a new spec, migration and pipeline, so it is **out of scope** here and listed as an open question.

---

## 4. Active train model (shared contract for positions, station board and train detail)

### 4.1 Effective times
For each non-cancelled stop, ordered by `actual_sequence_number`:
- `eff_arrival = actual_arrival ?? planned_arrival + (last known delay)`. The same rule gives `eff_departure`.
- `actual_*` on a **confirmed** stop is observed. On an unconfirmed stop it is PLK's forecast.
- If both `eff_arrival` and `eff_departure` are missing, the stop is skipped for interpolation.
- The first stop has no arrival. The last stop has no departure. Each uses the other time.

### 4.2 Active predicate (evaluated at time `t`, default `now()`)
All of the following must hold:
1. `train_status IN ('P','Q')`, or `train_status = 'S'` and `t ≥ eff_departure(first stop)`. The second case
   covers trains PLK has not flipped to P yet. It is tunable, and whether to include it is an open question.
2. `operating_date ∈ {local_today(t), local_today(t) − 1}`, using the Europe/Warsaw date, not UTC.
3. The train was seen in a recent snapshot: `updated_at ≥ max(train_operations.updated_at) − 15 min`. Trains that
   PLK stopped reporting disappear.
4. `eff_departure(first) − 5 min ≤ t ≤ eff_arrival(last) + 15 min grace`. This drops the "P for three days"
   zombies.

Align the exact thresholds with `fix/data-freshness`. They should be constants in one Go package.

### 4.3 Phase and position at time `t`
| Phase | Condition | Position |
|---|---|---|
| `not_departed` | `t < eff_departure(first)` | first stop |
| `at_station` | `eff_arrival(i) ≤ t ≤ eff_departure(i)` | stop `i` |
| `en_route` | `eff_departure(i) < t < eff_arrival(i+1)` | lerp between anchors, `progress = (t − dep(A)) / (arr(B) − dep(A))`, clamped to [0, 1] |
| `arrived` | `t > eff_arrival(last)` (within grace) | last stop |

- Anchors are A = the nearest stop ≤ i with coordinates and B = the nearest stop ≥ i+1 with coordinates.
- `position.method` is `station` or `interpolated` (A, B are the adjacent stops), `interpolated_sparse` (they are
  not adjacent), or `none` (no anchor, so `position` is null).
- `confidence` is one of:
  - `high`: the snapshot is under 15 min old and the last confirmation is under 20 min old;
  - `medium`: the snapshot is under 30 min old;
  - `low`: everything else.
- `delay_minutes` is the delay at the last confirmed stop (departure, falling back to arrival). If nothing is
  confirmed, it is null.

### 4.4 Wire shape: data-service `ActiveTrain` (snake_case, like the other data-service schemas)
```yaml
ActiveTrain:
  required: [operation_id, schedule_id, order_id, operating_date, train_status, phase, confidence]
  properties:
    operation_id: int64
    schedule_id: int
    order_id: int
    operating_date: date
    train_status: string            # S/P/Q
    route_name: string|null         # from routes, optional enrichment
    carrier_code: string|null
    origin:       StopRef           # {station_external_id, station_name}
    destination:  StopRef
    phase: enum [not_departed, at_station, en_route, arrived]
    previous_stop: StopTiming|null  # {station_external_id, station_name, sequence, time (eff departure), is_confirmed, latitude?, longitude?}
    next_stop:     StopTiming|null  # same, time = eff arrival
    last_confirmed_stop: StopTiming|null
    delay_minutes: int|null
    position: {latitude, longitude, progress (0..1), method} | null
    confidence: enum [high, medium, low]
    last_seen_at: date-time         # train_operations.updated_at
ActiveTrainListResponse:
  required: [data, generated_at, data_as_of]
  properties: {data: ActiveTrain[], generated_at: date-time, data_as_of: date-time, total: int}
```
`previous_stop` and `next_stop` carry the **segment anchor times and coordinates**. The frontend can therefore
dead-reckon a smooth motion between polls without new requests.

---

## 5. Implementation plan

Each step is small, is reviewable on its own and ships its own tests. Steps 0–2 can run in parallel. Step 3 needs 0
and 1. Step 5 needs 3. Step 6 needs 5 and should come after 2 for useful coverage.

| # | Step | Agent | Depends on |
|---|---|---|---|
| 0 | Fix the operation timestamp timezone (plugin + migration 010 + Go formatting) | data-engineering (+ go-backend for `FormatClock`) | — (coordinate with `fix/data-freshness`) |
| 1 | Live operations ingestion: DAG, collector pageSize, batched upsert, stale-lock guard | data-engineering (collector bit: go-backend) | — |
| 2 | Station coordinates from OSM | data-engineering | — |
| 3 | Contract: specs for the active-train endpoints | go-backend | — (land before 4/5) |
| 4 | Raw-lake retention for `raw/operations/` | devops-infra | 1 |
| 5 | data-service `GET /api/v1/operations/active` + estimator | go-backend | 0, 3 |
| 6 | gateway `GET /api/v1/map/trains` | go-backend | 3, 5 |
| 7 | Frontend trains layer | frontend-design | 6 (+ issue #11 for `src/lib`) |

---

### Step 0: Store operation timestamps as correct instants
**Agent:** `data-engineering`, with a small `go-backend` part.

> **Prompt.** In the Pociag do Predykcji repo, PLK `/api/v1/operations` returns naive local timestamps (e.g.
> `"plannedDeparture":"2026-09-28T10:30:00"`, Europe/Warsaw wall-clock). `airflow/plugins/pociag_processing/repository.py:39-42`
> (`_parse_timestamp`) returns a naive `datetime`, and Postgres (session TZ UTC) stores it as 10:30Z. Every
> `operation_stations` timestamp is therefore 1–2 h in the future. Fix it end to end:
> 1. In `repository.py`, make `_parse_timestamp` attach `ZoneInfo("Europe/Warsaw")` when the parsed value is naive,
>    and keep aware values unchanged. It is also used for carriers `validFrom/validTo`: keep the same semantics.
> 2. Add `db/migrations/010_operation_timestamps_warsaw.up.sql`/`.down.sql`. It is transactional. Up rewrites
>    `planned_arrival, planned_departure, actual_arrival, actual_departure` in `operation_stations` with
>    `(col AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw'`. Down does the inverse:
>    `(col AT TIME ZONE 'Europe/Warsaw') AT TIME ZONE 'UTC'`. Add a header comment saying the plugin fix must be
>    deployed together and the operations DAGs paused while it runs.
> 3. In `services/go/shared/trainutil/util.go` `FormatClock`, format in `Europe/Warsaw`: load it once, and add
>    `import _ "time/tzdata"` because the data-service image is Alpine. Otherwise the gateway's `HH:MM` fields shift
>    by 2 h.
> 4. Tests: add a `_parse_timestamp` unit test for naive, `Z` and offset inputs, and one for a DST date, in
>    `airflow/tests/test_repository.py`. Add a Go test for `FormatClock` in winter and summer.
>
> **Files:** `airflow/plugins/pociag_processing/repository.py`, `airflow/tests/test_repository.py`,
> `db/migrations/010_*.sql`, `services/go/shared/trainutil/util.go` (+ `util_test.go`).
>
> **Acceptance:**
> - After the migration, `SELECT count(*) FROM operation_stations os JOIN train_operations t ON t.id=os.train_operation_id WHERE os.is_confirmed AND os.actual_departure > t.updated_at + interval '5 min'` is ≈0.
> - The train detail page shows the same HH:MM as before.
> - `uv run ruff check . && uv run mypy plugins/pociag_processing && uv run pytest` pass.
> - `make data-service-test` and `make gateway-test` pass.
>
> Do not touch unrelated code. The spec schemas already say `date-time`, so no spec change is needed.

### Step 1: Near-real-time operations ingestion
**Agent:** `data-engineering` (collector constant: `go-backend`).

> **Prompt.** Operations are ingested once a day (`airflow/dags/ingest_operations_daily.py:57`, `0 2 * * *`), but
> the map needs positions at most about 10 min old. Implement the following:
> 1. **Spec first:** in `specs/airflow-dags.md`, add "DAG 5: `ingest_operations_live`": schedule `*/10 * * * *`,
>    catchup False, `max_active_runs=1`, `dagrun_timeout` 9 min, retries 0 (the next tick is the retry). Tasks:
>    `fetch_operations` → `process_operations`. Also correct DAG 3's purpose: the PLK endpoint is a real-time
>    snapshot, not "yesterday".
> 2. **New DAG** `airflow/dags/ingest_operations_live.py`. Mirror the TaskFlow style of `ingest_operations_daily.py`
>    (same `pociag_collector` connection, `_fetch_run_id` helper pattern; copy it, do not refactor the daily DAG).
>    Derive `capture_date` from the collector response's `lake_prefix` (`raw/operations/YYYY/MM/DD/`) rather than
>    `date.today()`, so the collector's UTC date and the worker's date cannot disagree. Leave the daily DAG as is.
> 3. **Collector:** in `services/go/collector/internal/service/service.go:384`, raise the operations `pageSize` from
>    1000 to 5000 (the PLK maximum per `specs/openapi/plk-open-data.json`). Update the existing collector tests.
> 4. **Faster upsert:** in `repository.py` `upsert_operations` (`:564-648`), replace the per-stop `cur.execute`
>    with `psycopg2.extras.execute_values` for `operation_stations`, keeping the same `ON CONFLICT` semantics. Keep
>    one transaction per call. The target is under 30 s for about 22 k trains and 380 k stops.
> 5. **Stale lock:** in `repository.is_pipeline_running` (`:63-81`), only count `running` rows with
>    `started_at > NOW() - interval '30 minutes'`. Make the equivalent change in the collector's `IsPipelineRunning`
>    (Go) only if it has the same problem, and keep it minimal.
>
> **Tests:** a unit test for the new DAG's `lake_prefix` → date parsing. Update the `test_repository.py`
> expectations for the batched upsert and the stale-lock SQL. Update the collector service test for the page size.
>
> **Acceptance:** with `make infra-up-all`, the new DAG appears unpaused-able in Airflow (:8090). A manual trigger
> finishes in under 5 min. `train_operations.updated_at` advances on each run. ruff, mypy (strict) and pytest are
> clean. `make collector-test` passes.
>
> Out of scope: disruptions cadence, NOTIFY events.

### Step 2: Station coordinates for the whole network
**Agent:** `data-engineering`.

> **Prompt.** Only 93 of 3,319 `stations` have `latitude/longitude`. They come from the hand-curated
> `airflow/plugins/pociag_processing/data/station_coordinates.json`, which is applied in
> `pipelines/dictionaries.py:100-106` via `repository.upsert_station_coordinates`. PLK's dictionary has no
> coordinates. Extend coverage **offline**:
> 1. Add a one-off script `airflow/scripts/build_station_coordinates.py`. It is not part of the plugin package and
>    has no runtime dependency. It:
>    - reads the PLK station list (a CSV/JSON export from `stations`, passed as an argument);
>    - queries Overpass for `node|way[railway~"^(station|halt)$"]` inside Poland;
>    - matches by normalised name (lowercase, strip diacritics, collapse punctuation, strip suffixes such as
>      "Główny/Gł.", "Osobowa/Os.");
>    - uses `city` / nearest-duplicate to break ties;
>    - writes `station_coordinates.json` in the **existing format** (`{source, stations:[{external_id, name,
>      latitude, longitude}]}`);
>    - always keeps existing hand-curated entries (they win);
>    - sets `source` to mention "© OpenStreetMap contributors, ODbL";
>    - prints a coverage report and writes the unmatched stations to stdout.
> 2. Regenerate the JSON and commit it. Do not change the schema, table or spec. Coordinates already exist in
>    `specs/schemas/station.json` and migration 009.
>
> **Tests:** unit-test the name-normalisation and matching functions in `airflow/tests/`.
>
> **Acceptance:**
> - At least 85 % of the distinct `station_external_id`s used by `operation_stations` in the last 7 days have
>   coordinates after a `sync_dictionaries_weekly` run.
> - Spot-check 20 random matches against the map.
> - ruff, mypy and pytest are clean.
>
> Out of scope: runtime geocoding, track geometry.

### Step 3: Contract: active trains (spec only)
**Agent:** `go-backend`.

> **Prompt.** Contract-first: add the specs for the train-positions feature. Change **only** `specs/`. Read
> `docs/proposals/train-positions.md` §4 for the model.
> 1. `specs/openapi/data-service.yml`: add `GET /api/v1/operations/active` (operationId `listActiveOperations`,
>    tag operations) with query params:
>    - `at` (date-time, optional, default now: used for replay and tests);
>    - `carrierCodes` (comma-separated);
>    - `limit` (default 2000, max 5000).
>
>    Response `ActiveTrainListResponse {data: ActiveTrain[], total, generated_at, data_as_of}`. Add the schemas
>    `ActiveTrain`, `StopRef`, `StopTiming`, `TrainPosition` exactly as in §4.4, snake_case, with
>    `additionalProperties: false`. Explain the active predicate and the effective-time rule in the `description`.
> 2. `specs/openapi/gateway.yml`: add `GET /api/v1/map/trains` (operationId `getMapTrains`, tag map) with the
>    optional query param `carriers`. Response `TrainMapResponse {trains: TrainMapPoint[], generated_at,
>    data_as_of}`. `TrainMapPoint` = `operation_id, train_name, carrier_code, status, phase, delay_minutes,
>    latitude, longitude, progress, method, confidence, previous_stop {station_name, time, latitude, longitude},
>    next_stop {…}, origin, destination`. Only trains with a non-null position are included; the response also
>    carries `unpositioned_count: int`. Times are RFC 3339 instants, not HH:MM, so the client can animate.
> 3. Validate both files (e.g. `npx @redocly/cli lint`, or whatever the repo uses; do not add new tooling
>    permanently).
>
> **Acceptance:** the specs lint. No code changes. The PR description lists the new schemas.

### Step 4: Raw operations retention
**Agent:** `devops-infra`.

> **Prompt.** A new `ingest_operations_live` DAG lands an operations snapshot every 10 min: about 125 MB of JSON
> each, about 18 GB/day under `s3://pociag-lake/raw/operations/`. Add a MinIO lifecycle (ILM) rule that expires
> objects under `raw/operations/` after `RAW_OPERATIONS_RETENTION_DAYS` days (default 3). Apply it where the bucket
> is created: the `minio-init` service in `infra/docker-compose.yml:30-37`, which runs `mc mb … local/pociag-lake`.
> Add the rule with `mc ilm rule add --prefix raw/operations/ --expire-days …` so it is idempotent. Document the
> variable in `infra/.env.example`.
>
> **Acceptance:** after `make infra-up-all`, `mc ilm rule ls local/pociag-lake` shows the rule. Other prefixes are
> untouched. No application code changes.

### Step 5: data-service active trains and position estimator
**Agent:** `go-backend`.

> **Prompt.** Implement `GET /api/v1/operations/active` in `services/go/data-service` exactly as specified in
> `specs/openapi/data-service.yml` (added in step 3). Design: `docs/proposals/train-positions.md` §4. Follow
> `services/go/data-service/CLAUDE.md`: raw SQL via `r.db.WithContext(ctx).Raw(...)`, `$n` placeholders, no GORM
> models.
> 1. **Estimator** (new package `internal/position`, pure, no DB):
>    - `EffectiveTimes(stops)`, `IsActive(op, stops, at, latestSnapshot) bool` and `Estimate(stops, at) Result`,
>      implementing §4.1–4.3;
>    - thresholds as named constants in one place (grace 15 min, pre-departure 5 min, snapshot window 15 min,
>      confidence cut-offs);
>    - operating dates computed in `Europe/Warsaw` (`import _ "time/tzdata"`).
>
>    The station-board and train-detail branches will reuse this package, so export only what those need.
> 2. **Repository** `ListActiveOperationRows(ctx, dates []time.Time, carrierCodes []string, limit int)` in
>    `internal/repository/operations.go`:
>    - one query over `train_operations` (filter `operating_date = ANY($1)`, `train_status IN ('S','P','Q')`,
>      `updated_at >= (SELECT MAX(updated_at) FROM train_operations) - interval '15 minutes'`);
>    - join `operation_stations` (non-cancelled, ordered by `actual_sequence_number`);
>    - `LEFT JOIN stations` for name and coordinates, `LEFT JOIN routes` for name and carrier;
>    - also return `MAX(updated_at)` as `data_as_of`;
>    - scan into row structs in `internal/model/model.go` and group in Go.
>
>    Do not N+1.
> 3. **Handler:**
>    - register `r.Get("/active", …)` inside the existing `/operations` route **before** `/{operationId}` (see the
>      "Static path before parametric" comment in `internal/handler/handler.go`);
>    - parse `at`, `carrierCodes`, `limit`;
>    - span name `operations.active`;
>    - update the swag annotations.
>
>    Add an in-memory cache of 15 s keyed by params, only when `at` is absent.
> 4. **Migration only if needed:** if `EXPLAIN ANALYZE` on real data (`make db-psql`) shows a seq scan on
>    `train_operations`, add `011_active_operations_index` with
>    `CREATE INDEX IF NOT EXISTS idx_train_operations_date_status_updated ON train_operations (operating_date, train_status, updated_at)`
>    (+ down).
> 5. **Tests:** table-driven unit tests for `internal/position` covering:
>    - at station;
>    - en route mid-segment;
>    - sparse anchors;
>    - no coordinates;
>    - first stop has no arrival and last stop has no departure;
>    - negative delay;
>    - cancelled intermediate stop;
>    - zombie `P` from two days ago (inactive);
>    - `S` past its departure;
>    - DST day;
>    - `progress` clamped.
>
>    Add a handler test for param validation (bad `at` → 400).
>
> **Acceptance:**
> - `make data-service-test` and `make data-service-lint` pass.
> - With the stack running, `curl localhost:8083/api/v1/operations/active | jq '.data|length'` returns a plausible
>   count (hundreds to low thousands in daytime).
> - p95 latency under 300 ms.
> - Positions of sampled trains lie between their previous and next station.

### Step 6: gateway `/api/v1/map/trains`
**Agent:** `go-backend`.

> **Prompt.** Implement `GET /api/v1/map/trains` in `services/go/gateway` per `specs/openapi/gateway.yml` (step 3).
> - Add `ListActiveOperations(ctx, params)` to the data-service client (`internal/client/dataservice`), calling
>   `GET /api/v1/operations/active`.
> - Map it to `TrainMapResponse` in `internal/service/service.go`, following `GetMapStations` (`:85-111`):
>   - span `trains.map`;
>   - `train_name` falls back to `"<carrier_code> <schedule_id>/<order_id>"` when `route_name` is missing;
>   - `status` uses `trainutil.StatusLabel`;
>   - drop entries with a null `position` and count them in `unpositioned_count`.
> - Register the route next to `/map/stations` in `internal/handler/handler.go` and add the swag annotations.
> - Do **not** change `/trains/live` in this step. Mention in the PR that it could reuse the active endpoint to
>   remove its N+1 (`service.go:340`).
>
> **Tests:** a service test with a fake client covering the mapping, the fallback name and the dropped
> unpositioned trains. A handler test for 200 and bad params.
>
> **Acceptance:** `make gateway-test` and `make gateway-lint` pass. `curl localhost:8084/api/v1/map/trains` returns
> trains with lat/lon inside Poland's bbox.

### Step 7: Frontend trains layer
**Agent:** `frontend-design`.

> **Prompt.** Show live trains on the network map in `services/frontend`. Shape: `TrainMapResponse` in
> `specs/openapi/gateway.yml`.
>
> **Caveat:** `src/lib/` is currently git-ignored (issue #11). Make sure `src/lib/api.ts` changes are actually
> committed; coordinate with the fix for #11.
> 1. In `src/lib/api.ts`, add the `TrainMapPoint`/`TrainMapResponse` interfaces and `gateway.getMapTrains(carriers?)`.
> 2. In `src/components/TrafficMapClient.tsx`, add a `trainsPane` (zIndex 430, above `stationsPane`) that renders
>    trains:
>    - `useQuery(['mapTrains'], …, { refetchInterval: 30_000, staleTime: 20_000 })`;
>    - a small marker coloured by delay: on time ≤ 0 green, 1–5 amber, > 5 red, null grey;
>    - reduced opacity for `confidence=low`;
>    - `Popup`: train name, carrier, previous → next station with times, delay, "dane z HH:MM" from `data_as_of`,
>      and a link to `/pociagi/{operation_id}`.
> 3. **Smooth motion:** every 5 s, recompute each en-route train's position client-side by lerping
>    `previous_stop` → `next_stop` using their instants and `Date.now()`, clamped to [0, 1]. Server data replaces it
>    on each refetch.
> 4. Add a layer toggle ("Stacje" / "Pociągi") and a small legend and counter ("N pociągów, M bez pozycji").
> 5. With about 3 k stations now having coordinates, show the permanent station name tooltips only at zoom ≥ 9, to
>    keep the map readable and fast.
> 6. Handle loading and error states: the trains layer must not break the station map when the endpoint fails.
>
> **Acceptance:**
> - `npm run build` passes.
> - With the stack running, the `/mapa` page shows moving train markers in daytime.
> - Popups open.
> - The toggle works.
> - No console errors.
> - Performance is acceptable with 2 k markers (use `CircleMarker`/canvas renderer, `preferCanvas`).
>
> Do not restyle unrelated parts of the page.

---

## 6. Open questions and decisions for the user
1. **PLK API tier.** Which rate-limit tier is our key on? A 10-min cadence (~720 calls/day plus other DAGs) is
   tight on Basic (1,000/day). 5 min needs Standard. (`GET /api/v1/apikey/info` will tell.)
2. **Cadence and retention.** Is 10 min / 3-day raw retention acceptable? Or do you want 5 min, and/or to keep one
   snapshot per day permanently for ML (ADR-002 mentions future ML)?
3. **Timezone fix ownership.** Step 0 in this branch or in `fix/data-freshness`? It changes displayed data
   semantics and needs a coordinated deploy (pause DAGs → migrate → deploy plugin + gateway).
4. **Which trains count as active.** Include `S` trains whose planned departure has passed (PLK is slow to flip to
   `P`)? Show `arrived` trains during the 15-min grace? Include `Q` (partially cancelled)?
5. **OSM data licence.** Is ODbL attribution for station coordinates acceptable? The map already shows OSM/ORM
   attribution.
6. **Track-following positions** (§3.5). Is this worth a follow-up phase with a `station_segments` geometry
   table, or are straight lines fine?
7. **Carrier/category filter on the map.** Is it needed now (the specs include a `carriers` param), or later?
8. **`/trains/live`.** Should it be migrated onto the active endpoint (removes N+1, consistent "active" rule) as
   part of this feature or by `feature/train-details`?
