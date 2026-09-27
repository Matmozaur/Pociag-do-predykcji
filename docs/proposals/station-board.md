# Proposal: Station board ("tablica stacyjna")

Status: **proposal**. Nothing here is implemented yet. Branch: `feature/station-board`.

## The request

> I want a view for stations with data like trains currently at the station, trains about to
> arrive, and trains about to depart. I want to be able to open it by clicking on the station
> (on the map).

## TL;DR

- **The data supports a board.** `operation_stations` has planned, actual/expected times,
  delays, confirmation and cancellation flags for each stop of each train on each day. I
  prototyped the board query against the local DB for Warszawa Zachodnia and it returned
  plausible rows in about 87 ms. That time grows as history accumulates, so we should add one index.
- **Two data problems block a *correct* board, and they should be fixed first or together
  with `fix/data-freshness`:**
  1. **Timestamps are Polish local time stored as if they were UTC** (evidence in §2.4). "Now"
     comparisons are 2 h off unless we account for that. The UI shows the right HH:MM only
     because nothing converts time zones anywhere.
  2. **Operations are ingested once a day** (`0 2 * * *`), so delays and "at station" states are
     a morning snapshot. The board will be a *timetable with the morning's delay forecast*
     until ingestion runs during the day.
- There is also a local-environment gap. `routes` (train name, category, platform) was last
  loaded on 2026-07-19, so only 1 of today's 2,340 operations joins to a route. The board has
  to degrade gracefully, and the schedules DAG has to run.
- **Design:** a new data-service endpoint `GET /api/v1/stations/{externalId}/board`
  returns candidate stop events for a time window, and a pure Go function sorts them into
  buckets. A new gateway endpoint `GET /api/v1/stations/{externalId}/board` shapes this into
  `at_station` / `arrivals` / `departures` for the frontend. On the frontend, clicking a station
  on the map opens a **board panel** in the existing map side panel or bottom sheet, with state
  in the URL (`/mapa?stacja=33506`). The same `StationBoard` component also backs a full page
  at `/stacje/[id]`. Each row links to `/pociagi/{operation_id}` (the train view, which
  `feature/train-details` owns).

---

## 1. Current state (evidence)

### 1.1 Map and station markers

- Stations render as `CircleMarker`s in `stationsPane`:
  `services/frontend/src/components/TrafficMapClient.tsx:145-170`.
- They are **already clickable, but a click only opens a Leaflet popup** with name, city and
  external ID: `TrafficMapClient.tsx:152-154` (`click: (e) => e.target.openPopup()`) and
  `StationPopupContent` at `:69-79`. Nothing navigates anywhere.
- Station labels are permanent tooltips (`:156-164`).
- Stations come from `gateway.getMapStations` → `GET /api/v1/map/stations`, with a 1 h
  `staleTime` (`TrafficMapClient.tsx:84-88`).
- The map lives inside `MapHomeClient` (`services/frontend/src/components/MapHomeClient.tsx:86-126`).
  Its "Centrum operacyjne" panel is a left side card on desktop and a bottom sheet on mobile
  (`:90`). That panel is the natural place for a station board.
- `/mapa` is the home page (`services/frontend/src/app/page.tsx` redirects to `/mapa`;
  `app/mapa/page.tsx` dynamically imports `MapHomeClient` with `ssr:false`).
- **Only 93 of 3,319 stations have coordinates** (local DB), so only 93 stations are clickable
  today. The board itself works for any station `external_id`. Station search
  (`/wyszukaj`) could be a second entry point.

### 1.2 Existing station, operations and train endpoints

| Layer | Endpoint | Where |
|---|---|---|
| gateway | `GET /api/v1/search/stations` | `specs/openapi/gateway.yml:61`, `handler.go:41` |
| gateway | `GET /api/v1/map/stations` | `gateway.yml:95`, `handler.go:42`, `service.go:86-109` |
| gateway | `GET /api/v1/trains/live?stations=` | `gateway.yml:204`, `service.go:322-373` (only status `P`, "today" in UTC, **N+1** `GetOperationByID` per row) |
| gateway | `GET /api/v1/trains/{operationId}` | `gateway.yml:238`, `service.go:375-434` (the train view the board rows will link to) |
| data-service | `GET /api/v1/stations`, `/stations/{externalId}` | `data-service.yml:381,418`, `handler.go:58-59` |
| data-service | `GET /api/v1/operations?stationExternalIds=&date=` | `data-service.yml:223`, `repository/operations.go:15-154`. Filters operations that *touch* a station, but returns no per-stop times |
| data-service | `GET /api/v1/operations/{id}` | `operations.go:156-297`. Per-stop times for **one** train |

**No existing endpoint answers "which trains are at, or about to arrive at or depart from,
station X around time T".** `trains/live?stations=` is the closest match, but it only covers
status `P`, has no per-station times, and costs N+1 calls. It is not a basis for a board.

The frontend already has `/pociagi/[id]` (train detail by `operation_id`) at
`services/frontend/src/app/pociagi/[id]/page.tsx`. Board rows can link to it.

### 1.3 Curated tables

(The task brief says `route_stops`; the real table is `route_stations`.)

| Table | Board-relevant columns | Migration |
|---|---|---|
| `train_operations` | `id, schedule_id, order_id, operating_date, train_status (S/P/C/X/Q), updated_at` | `003_operations.up.sql:8-19` |
| `operation_stations` | `station_external_id, planned_sequence_number, actual_sequence_number, planned_arrival/departure, actual_arrival/departure (timestamptz), arrival/departure_delay_minutes, is_confirmed, is_cancelled` | `003_operations.up.sql:26-43` |
| `routes` | `name, carrier_code, national_number, commercial_category_symbol`, key `(schedule_id, order_id)` | `002_schedules.up.sql:8-22` |
| `route_stations` | `arrival/departure_platform`, `arrival/departure_track`, `arrival/departure_train_number`, key `(route_id, order_number)` | `002_schedules.up.sql:43-64` |
| `stations` | `external_id, name, city, latitude, longitude` | `001`, `009` |
| `carriers` | `code, name` | `001` |

Joins that I validated against the DB:

- `operation_stations` → `train_operations` on `train_operation_id`.
- `train_operations` → `routes` on `(schedule_id, order_id)` (the same join `operations.go:74-75` uses).
- `operation_stations.planned_sequence_number = route_stations.order_number` (same `route_id`).
  Platforms, tracks and train numbers line up on 2026-07-25 data.
- Origin and destination are the first and last `actual_sequence_number` of the operation.

Existing indexes: `idx_operation_stations_station (station_external_id)` only
(`003_operations.up.sql:40`). There is nothing on time.

### 1.4 Data quality observed in the local DB (2026-09-27, read-only)

| Fact | Value | Impact on board |
|---|---|---|
| `operation_stations` rows | 2.87 M, operating dates 2026-07-23 to 2026-09-28 | Fine |
| Warszawa Zachodnia stop rows (all history) | ~28 k | Station-only index scans all of them. Prototype took **87 ms** and grows linearly |
| `actual_arrival`/`actual_departure` populated | ~100 % of rows that have planned times, **including unconfirmed future stops** | PLK fills "actual" with *expected* (planned + propagated delay). Confirmed on 40,242 / 40,804 rows today: `actual = planned + delay` |
| `actual_arrival` present where `planned_arrival` is NULL | origin stops | Must derive "has an arrival" from `planned_arrival`, not `actual_arrival` |
| `is_confirmed` today | 18,422 / 42,845 stop rows | Only as fresh as the last ingestion |
| last `train_operations.updated_at` | 2026-09-27 06:13 UTC | Delays and states on the board are about 7 h stale this afternoon |
| status of trains on 2026-09-25 | 211 `P`, 420 `S` still open | Past days never finalize (belongs to fix/data-freshness) |
| `routes` last updated | 2026-07-19 (`ingestion_runs` schedules last run 2026-07-19) | **Only 1 of 2,340 of today's operations joins to a route** → no name, category, carrier or platform |
| `routes.name` non-null | 6,915 / 19,330 (regional trains have no name) | Need a display fallback: `category + national_number` |
| `route_stations.arrival_time`/`departure_time` non-null | **0 / 382,861** | Not needed by the board (it uses `operation_stations`), but see OTHER FINDINGS |
| `route_stations.departure_platform` non-null | 326 k / 383 k | Platform available where routes are fresh |

### 1.5 Timezone handling today

- **Stored timestamps are Polish wall-clock times labelled UTC.** For operating date
  2026-09-26, `min(planned_departure) = 2026-09-26 00:00:00+00` and
  `max(planned_arrival) = 2026-09-26 23:59:54+00`, and **zero rows fall before UTC midnight**.
  If these were true UTC instants, a Polish operating day would start at 22:00 UTC the day
  before. A second check: trains planned at "05:57Z" were already status `C` in the
  06:13 UTC (= 08:13 CEST) snapshot, which only makes sense if 05:57 is local time.
  The source is `_parse_timestamp` → `datetime.fromisoformat(value)`
  (`airflow/plugins/pociag_processing/repository.py:39-42`). Either the PLK string is naive,
  and becomes UTC through the session `TimeZone=UTC`, or it carries a misleading `Z`.
  **This needs confirming on one raw Parquet file** (open question Q1).
- The gateway formats clocks with `ts.Format("15:04")` and no zone conversion
  (`services/go/shared/trainutil/util.go:51-57`). The current UI therefore shows the *correct*
  wall-clock only by accident. If the data is fixed without also fixing `FormatClock`, the UI
  will be 1–2 h off.
- "Today" is computed in UTC in the gateway (`gateway/internal/service/service.go:326,439,541`)
  and in the process-local zone in data-service (`data-service/internal/handler/operations.go:43,115`).
  Between 00:00 and 02:00 CEST the wrong operating day is selected.
- Postgres session TZ is `UTC`. The data-service image is `alpine` and the gateway image is
  `distroless`. Neither is guaranteed to ship tzdata, so Go code must `import _ "time/tzdata"`
  before calling `time.LoadLocation("Europe/Warsaw")`.

### 1.6 Pipeline freshness

- `ingest_operations_daily` runs `schedule="0 2 * * *"` (`airflow/dags/ingest_operations_daily.py:57`).
  It takes one snapshot per day for the capture date, then disruptions in the same DAG. So
  the "4 DAGs" in the root CLAUDE.md are really 3.
- `ingest_schedules_weekly` is `0 4 * * 1` (`airflow/dags/ingest_schedules_weekly.py:27`).
- There is no intraday operations refresh, which is the core of what `fix/data-freshness` and
  `feature/train-positions` also need.

### 1.7 Gateway caching today

- Only a global `Cache-Control` header from env `CACHE_CONTROL` (default `no-store`), set via
  `middleware.CacheHeaders` (`gateway/cmd/main.go:77`, `internal/config/config.go:43`).
  There is no in-process response cache.
- The frontend's `apiFetch` sets `next: { revalidate: 30 }`, which only matters server-side.
  The client relies on react-query `staleTime`/`refetchInterval`.

---

## 2. Design

### 2.1 Definitions (the board semantics)

For station `S`, reference instant `now`, and window `W` (default 60 min):

For each stop row `os` at `S` whose operation is not fully cancelled (`train_status <> 'X'`),
compute:

```
has_arr  = planned_arrival   IS NOT NULL          -- false at origin
has_dep  = planned_departure IS NOT NULL          -- false at terminus
exp_arr  = CASE WHEN has_arr THEN COALESCE(actual_arrival,
             planned_arrival + arrival_delay_minutes * interval '1 minute', planned_arrival) END
exp_dep  = CASE WHEN has_dep THEN COALESCE(actual_departure,
             planned_departure + departure_delay_minutes * interval '1 minute', planned_departure) END
```

Buckets are assigned in Go by a pure function, so they are unit-testable without a DB:

| Bucket | Rule |
|---|---|
| `at_station` | not `is_cancelled`, `has_arr`, `exp_arr <= now`, and (`has_dep` and `exp_dep > now`, **or** terminus and `exp_arr > now - 5 min`) |
| `arrivals` | `has_arr` and `now < exp_arr <= now + W` |
| `departures` | `has_dep` and `now < exp_dep <= now + W`, and **not** in `at_station` |

- A through train that arrives in 5 min and leaves in 7 min appears in **both** arrivals and
  departures, as on a real station board. A train that is standing at the platform appears
  only in `at_station`, which shows its departure time. (Alternative in Q3.)
- Cancelled stops (`is_cancelled`) stay in arrivals and departures, flagged, and never go in `at_station`.
- Lookback: SQL only reads stops with `COALESCE(planned_departure, planned_arrival)` in
  `[now - 6h, now + W]`. The 6 h lower bound covers delays up to 6 h. Filtering by time
  instead of `operating_date` naturally handles trains that run past midnight on the previous
  operating day.
- Freshness caveat: `now` is compared with *expected* times as last ingested. With daily
  ingestion, a train that has since picked up extra delay still shows the morning's forecast.
  The response carries `data_as_of` so the UI can say so.

### 2.2 The "now" / timezone contract (shared with fix/data-freshness and train-positions)

The board **must not** re-invent this. Two options:

| Option | What | Pros | Cons |
|---|---|---|---|
| **A. Fix at ingestion (recommended)** | Plugin reads PLK times as `Europe/Warsaw`, and a one-off migration reinterprets existing rows. `trainutil.FormatClock` converts to `Europe/Warsaw`. A new `trainutil.ServiceDate(now)` returns the Warsaw calendar date for all "today"s | Stored data is correct. SQL can use `now()`. Every consumer is right by construction | Touches the plugin, a migration and shared Go. Must land atomically with the `FormatClock` change or the UI shifts by 2 h |
| B. Compensate at read time | data-service passes `now` as "Warsaw wall-clock expressed as UTC" (`time.Date(y,m,d,h,mi,s,0,time.UTC)` from `time.Now().In(warsaw)`) | Tiny, local change | Every new query must remember the trick. DST edge cases (02:00–03:00 on the autumn change) are ambiguous. It perpetuates wrong data |

**Recommendation:** A, owned by `fix/data-freshness` (it already has to fix "ended routes shown
as current", which is the same "now" problem). The board should consume `now` through **one
function in data-service** (`boardNow(r *http.Request) time.Time`, which honours an optional
`at` query param for tests and debugging). If the board ships before A lands, only that
function carries the option B shim, marked `// TODO(fix/data-freshness)`, and it is removed
when A lands.

### 2.3 API (contract-first)

#### data-service: `GET /api/v1/stations/{externalId}/board`

Query: `at` (RFC 3339 date-time, optional, default now), `windowMinutes` (int, 5–240,
default 60), `lookbackMinutes` (int, 0–720, default 360). Tag `operations`.

`200` → `StationBoardResponse`:

```yaml
StationBoardResponse:
  required: [station_external_id, at, window_minutes, entries]
  properties:
    station_external_id: integer
    at:              date-time        # the reference instant actually used
    window_minutes:  integer
    data_as_of:      date-time        # max(train_operations.updated_at) over returned ops (nullable)
    entries: [StationBoardEntry]      # already bucketed, ordered by board time
StationBoardEntry:
  required: [operation_id, schedule_id, order_id, operating_date, train_status,
             sequence_number, bucket, is_confirmed, is_cancelled]
  properties:
    bucket: enum [at_station, arrival, departure]
    operation_id, schedule_id, order_id, train_order_id, operating_date, train_status
    sequence_number                     # actual_sequence_number
    route_id                            # nullable (routes may be missing)
    route_name, carrier_code, commercial_category, national_number   # nullable
    train_number                        # route_stations.departure_train_number ?? arrival_train_number
    platform, track                     # departure_* for departure/at_station, arrival_* for arrival
    planned_arrival, planned_departure, expected_arrival, expected_departure   # date-time, nullable
    arrival_delay_minutes, departure_delay_minutes                          # nullable
    is_confirmed, is_cancelled
    origin_station_external_id, origin_station_name
    destination_station_external_id, destination_station_name
```

`404` if the station `externalId` does not exist. `400` for bad params.

One entry per (stop, bucket), so a through train can yield an `arrival` and a `departure`
entry. The data-service does the bucketing, so the gateway stays a pure shaper and the rules
are tested in one place.

#### gateway: `GET /api/v1/stations/{externalId}/board`

Query: `window` (int minutes, allowed 30|60|120, default 60), `limit` (per bucket, default 20,
max 50). Tag `stations` (add it to `tags:`).

`200` → `StationBoardView`:

```yaml
StationBoardView:
  required: [station, generated_at, window_minutes, at_station, arrivals, departures]
  properties:
    station: { external_id, name, city?, latitude?, longitude? }
    generated_at:   date-time
    data_as_of:     date-time          # for "Dane operacyjne z HH:MM" + staleness warning
    window_minutes: integer
    at_station: [StationBoardRow]
    arrivals:   [StationBoardRow]
    departures: [StationBoardRow]
StationBoardRow:
  required: [operation_id, train_name, status, is_cancelled, is_confirmed]
  properties:
    operation_id: int64                # link → /pociagi/{operation_id}
    train_name:   string               # route_name ?? "<category> <national_number>" ?? "Pociąg <train_number>" ?? "Pociąg <schedule_id>/<order_id>"
    train_number, commercial_category
    carrier: { code, name? }
    origin, destination                # station names
    planned_time:  "HH:MM"             # arrival for arrivals; departure for departures/at_station
    expected_time: "HH:MM"
    delay_minutes: integer
    minutes_until: integer             # vs generated_at (negative for at_station arrivals)
    platform, track
    status: enum [not_started, in_progress, completed, cancelled, partial_cancelled]
    is_cancelled, is_confirmed
```

Gateway fan-out: `GetStationByExternalID` + new `GetStationBoard` + `ListCarriers` (the
existing `loadCarrierMap`, `service.go:117`). That is 3 calls in total, with no N+1.

### 2.4 SQL (data-service repository)

Validated shape (prototype run on the local DB, 27 rows for Warszawa Zachodnia).
Placeholders: `$1` station, `$2` now, `$3` window interval, `$4` lookback interval.

```sql
WITH cand AS (
  SELECT t.id AS operation_id, t.schedule_id, t.order_id, t.train_order_id,
         t.operating_date, t.train_status, t.updated_at,
         os.actual_sequence_number, os.planned_sequence_number,
         os.planned_arrival, os.planned_departure,
         CASE WHEN os.planned_arrival IS NOT NULL THEN COALESCE(os.actual_arrival,
              os.planned_arrival + make_interval(mins => COALESCE(os.arrival_delay_minutes, 0))) END AS exp_arr,
         CASE WHEN os.planned_departure IS NOT NULL THEN COALESCE(os.actual_departure,
              os.planned_departure + make_interval(mins => COALESCE(os.departure_delay_minutes, 0))) END AS exp_dep,
         os.arrival_delay_minutes, os.departure_delay_minutes, os.is_confirmed, os.is_cancelled
  FROM operation_stations os
  JOIN train_operations t ON t.id = os.train_operation_id
  WHERE os.station_external_id = $1
    AND COALESCE(os.planned_departure, os.planned_arrival) BETWEEN $2 - $4 AND $2 + $3
    AND t.train_status <> 'X'
)
SELECT c.*,
       r.id AS route_id, r.name, r.carrier_code, r.commercial_category_symbol, r.national_number,
       COALESCE(rs.departure_train_number, rs.arrival_train_number) AS train_number,
       rs.arrival_platform, rs.arrival_track, rs.departure_platform, rs.departure_track,
       o.station_external_id AS origin_id,  so.name AS origin_name,
       d.station_external_id AS dest_id,    sd.name AS dest_name
FROM cand c
LEFT JOIN routes r          ON r.schedule_id = c.schedule_id AND r.order_id = c.order_id
LEFT JOIN route_stations rs ON rs.route_id = r.id AND rs.order_number = c.planned_sequence_number
LEFT JOIN LATERAL (SELECT station_external_id FROM operation_stations
                   WHERE train_operation_id = c.operation_id
                   ORDER BY actual_sequence_number ASC  LIMIT 1) o ON TRUE
LEFT JOIN LATERAL (SELECT station_external_id FROM operation_stations
                   WHERE train_operation_id = c.operation_id
                   ORDER BY actual_sequence_number DESC LIMIT 1) d ON TRUE
LEFT JOIN stations so ON so.external_id = o.station_external_id
LEFT JOIN stations sd ON sd.external_id = d.station_external_id
ORDER BY COALESCE(c.exp_dep, c.exp_arr);
```

- Go then filters and buckets the rows (§2.1) and truncates each bucket.
- The lateral lookups use the existing `uq_operation_station (train_operation_id, actual_sequence_number)`.
- **Index (migration `010_operation_stations_station_time`):**
  ```sql
  CREATE INDEX IF NOT EXISTS idx_operation_stations_station_planned_time
      ON operation_stations (station_external_id, (COALESCE(planned_departure, planned_arrival)));
  ```
  The predicate in the SQL must use exactly this expression. Down: `DROP INDEX IF EXISTS`.
  This turns the ~28 k-row bitmap scan per busy station into a range scan of a few hundred rows.
  Leave `idx_operation_stations_station` in place; `QueryOperations` still uses it.

### 2.5 Gateway caching

- **v1: no server-side cache.** With the index, the query should be a few ms. The client
  refreshes every 60 s. Per-station, per-minute load is tiny, and the rate limiter already
  exists.
- Keep the global `Cache-Control` (`no-store`). A per-route `max-age=30` would need
  middleware changes that go beyond this feature (Q6).
- If load ever matters, add a 30 s in-process TTL cache keyed by `(station, window, minute)`
  in `internal/service`, since data only changes on ingestion anyway. That is deferred.

### 2.6 Frontend UX

| Option | Pros | Cons |
|---|---|---|
| Popup content only | Smallest | Leaflet popups are cramped, hard to scroll, bad on mobile |
| **Panel in map + `/stacje/[id]` page (recommended)** | Keeps map context. Mobile already uses a bottom sheet (`MapHomeClient.tsx:90`). URL state gives back button and sharing. The full page gives room and works from search | Two containers for one component (small cost) |
| Route only (`/stacje/[id]`) | Simplest routing | Loses the map, and needs a full navigation for each station click |

Recommended flow:

1. Clicking a `CircleMarker` calls `onStationSelect(station)` instead of `openPopup()`
   (`TrafficMapClient.tsx:152-154`). Drop the popup and keep the permanent label tooltip.
   Highlight the selected marker (larger radius, different colour) and pan to it.
2. `MapHomeClient` keeps `selectedStationId` in the URL (`/mapa?stacja=<external_id>`,
   `useSearchParams` + `router.replace`). When it is set, the side panel or bottom sheet shows
   `<StationBoard stationId=… variant="panel" />` with a back ("← Przegląd sieci") and a
   close button. When it is not set, the panel shows the existing overview. `Esc` closes it.
3. New route `src/app/stacje/[id]/page.tsx` shows `NavShell` + `<StationBoard variant="page" />`.
   The panel has a "Pełna tablica" link to it.
4. `StationBoard` contents:
   - Header with station name, city, and a window selector (30 / 60 / 120 min).
   - A freshness line: "Zaktualizowano HH:MM · dane operacyjne z HH:MM". It turns into an
     amber warning when `data_as_of` is more than 30 min old, e.g. "Opóźnienia mogą być
     nieaktualne".
   - Three sections: **Na stacji**, **Przyjazdy**, **Odjazdy**. Tabs on mobile, stacked on
     desktop and page.
   - Row layout: time (planned, with expected struck through when delayed), train name and
     number, a carrier badge, "z {origin}" for arrivals or "do {destination}" for departures,
     platform/track, a delay `Badge` (reusing `delayVariant`/`formatDelay`), a "za N min"
     label, and a cancelled style. The whole row is a `Link` to `/pociagi/{operation_id}`,
     following the `TrainRow` pattern in `MapHomeClient.tsx:48-62`.
   - States: skeleton rows while loading; an error card with "Spróbuj ponownie" (the
     existing `QueryMessage` pattern); a per-section empty state ("Brak przyjazdów w
     najbliższych 60 min"); and a 404 "Nie znaleziono stacji".
5. Data: a `useStationBoard(id, window)` react-query hook in `src/lib/` with
   `queryKey: ['stationBoard', id, window]`, `staleTime: 30_000`,
   `refetchInterval: 60_000`, and `refetchIntervalInBackground: false`.
   `minutes_until` is recomputed client-side every 30 s from `generated_at`, so the labels
   tick without refetching.
6. Accessibility: the panel is a `section` with `aria-labelledby`, focus moves to its heading
   when it opens, and the `Esc` key closes it.

⚠ `services/frontend/src/lib/` is currently git-ignored (open issue **#11**). `api.ts` changes
cannot be committed until #11 is fixed, or unless the file is force-added. The frontend step
depends on this.

### 2.7 Shared pieces and sibling-branch dependencies

| Shared piece | Owner (proposed) | Used by |
|---|---|---|
| "now"/timezone contract (§2.2): ingestion fix + `trainutil.FormatClock` in Warsaw + `trainutil.ServiceDate` | **fix/data-freshness** | board, train-positions, train-details, dashboard |
| Intraday operations ingestion (e.g. every 10 min for today's capture date) | fix/data-freshness or train-positions (data-eng + devops) | board delays, train positions, live list |
| "Expected time" rule `COALESCE(actual, planned + delay, planned)` and "has arrival = planned_arrival not null" | board defines it here (§2.1). train-positions should reuse the same expressions or Go helper | board, train-positions (position between last-departed and next-arrival stop) |
| Train display name fallback (`name ?? category + national_number ?? …`) | board adds `trainutil.DisplayName(...)` in `shared` | board, live trains, train-details |
| `/pociagi/[id]` train view | **feature/train-details** | board rows link to it. Only the URL contract (`operation_id`) is shared, so no blocking dependency |
| Map components `TrafficMapClient.tsx` / `MapHomeClient.tsx` | board changes the marker click and the panel. **train-positions** will add train markers or a layer | Merge conflicts are likely. Keep the board's diff to the click handler, a selected-marker prop, and a panel switch. Agree that train-positions adds a separate `<Pane>` |

Sequencing: time contract (fix/data-freshness) → board spec + migration → data-service →
gateway → frontend. Train-positions can run in parallel and should rebase on the time
contract too.

### 2.8 Out of scope (v1)

Platform-change detection, connection info (`route_connections`), disruptions for the station
(the `queryDisruptions?stationExternalIds=` endpoint exists and could become a banner later),
historical boards (the `at` param allows it, but there is no UI), push updates.

---

## 3. Implementation steps

Each step is one small PR-sized commit on `feature/station-board`, or a sibling branch where noted.
The prompts are self-contained.

### Step 0 — Time contract (prerequisite; coordinate with `fix/data-freshness`)

Skip this step if `fix/data-freshness` has landed an equivalent fix. Otherwise it belongs on
that branch, not here.

**Subagent:** `data-engineering` (0a), then `go-backend` (0b). Both must merge together.

**Prompt 0a (data-engineering):**
> In the Pociag do Predykcji repo, operation timestamps are PLK local wall-clock times
> stored in TIMESTAMPTZ as if they were UTC. Evidence: for operating_date 2026-09-26,
> `min(planned_departure)=2026-09-26 00:00Z` and no rows fall before UTC midnight.
> `_parse_timestamp` in `airflow/plugins/pociag_processing/repository.py:39-42` uses
> `datetime.fromisoformat` with no zone handling. First, open one raw operations Parquet file
> from MinIO (`s3://pociag-lake/raw/`, via `LakeReader`) and record the exact string format
> of `plannedArrival`/`actualArrival`: naive, `Z`, or an offset. Then:
> (1) Change `_parse_timestamp` so that naive values (and `Z` values, if PLK's `Z` is proven
> to be misleading) are interpreted in `ZoneInfo("Europe/Warsaw")`. Keep genuine offsets
> as they are.
> (2) Add migration `db/migrations/010_operation_times_to_warsaw.{up,down}.sql`, or the
> next free number, that converts existing `operation_stations.planned_*/actual_*` with
> `(col AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw'`. The down migration is the inverse.
> Wrap it in a transaction and document that it is not idempotent (guard with a marker
> table or comment clearly).
> Tests: add pytest cases in `airflow/tests/` for naive, `Z` and offset inputs. `ruff`,
> `mypy --strict` and `pytest` must pass. Follow `airflow/CLAUDE.md` and
> `db/migrations/CLAUDE.md`. Do not touch anything else. Report the observed raw format.
>
> Acceptance: after the migration, `min(planned_departure)` for a CEST operating date is
> 22:00Z the previous day. All tests pass.

**Prompt 0b (go-backend):**
> In `services/go/shared/trainutil/util.go`, `FormatClock` formats `ts.Format("15:04")` with
> no zone conversion. After the ingestion fix, stored times are true UTC instants. Change it
> to convert to `Europe/Warsaw`. Load the location once with a package-level
> `time.LoadLocation`, and add `import _ "time/tzdata"` because the images are alpine and
> distroless. Add `ServiceDate(now time.Time) string` returning the Warsaw calendar date
> `YYYY-MM-DD`. Replace the "today" computations at
> `services/go/gateway/internal/service/service.go:326,439,541` and
> `services/go/data-service/internal/handler/operations.go:43,115` with it. Add table tests
> in `shared/trainutil/util_test.go` covering CEST, CET, and 23:30Z → next Warsaw day.
> `make gateway-test data-service-test` and lint must pass. Change nothing else.

### Step 1 — Contract: spec changes

**Subagent:** `go-backend`.

**Prompt:**
> Contract-first spec change for a station board feature in Pociag do Predykcji. Read
> `docs/proposals/station-board.md` §2.1 and §2.3 and follow them exactly.
> (1) In `specs/openapi/data-service.yml`, add `GET /api/v1/stations/{externalId}/board`
> (operationId `getStationBoard`, tag `operations`, params `at` date-time,
> `windowMinutes` 5–240 default 60, `lookbackMinutes` 0–720 default 360, and responses
> 200 `StationBoardResponse`, 400, 404). Add schemas `StationBoardResponse` and
> `StationBoardEntry` with `additionalProperties: false`, matching the proposal field list.
> Timestamps are `format: date-time` and nullable fields are optional.
> (2) In `specs/openapi/gateway.yml`, add `GET /api/v1/stations/{externalId}/board`
> (operationId `getStationBoard`, params `window` enum [30,60,120] default 60 and `limit`
> 1–50 default 20, responses 200 `StationBoardView`, 400, 404). Add schemas
> `StationBoardView` and `StationBoardRow`, and a `stations` tag in the `tags:` list.
> Bump `info.version` minor in both files. Only edit these two files. Validate that both
> parse (for example `npx @redocly/cli lint` or a YAML load). Do not edit code.
>
> Acceptance: both specs lint and parse, and no other file is changed.

### Step 2 — Index migration

**Subagent:** `data-engineering`.

**Prompt:**
> Add migration `db/migrations/0NN_operation_stations_station_time.up.sql` / `.down.sql`,
> using the next free number (010, or 011 if Step 0a took 010). Up:
> `CREATE INDEX IF NOT EXISTS idx_operation_stations_station_planned_time ON operation_stations (station_external_id, (COALESCE(planned_departure, planned_arrival)));`
> in `BEGIN/COMMIT`. Down: `DROP INDEX IF EXISTS idx_operation_stations_station_planned_time;`.
> Follow `db/migrations/CLAUDE.md`. Apply with `make db-migrate-up`, roll back with
> `make db-migrate-down`, then apply again. Verify with `EXPLAIN ANALYZE` that
> `WHERE station_external_id = 33506 AND COALESCE(planned_departure, planned_arrival) BETWEEN now()-interval '6 hours' AND now()+interval '1 hour'`
> uses the new index, and report the before and after timings. Touch nothing else.
>
> Acceptance: up/down/up is clean, and the plan shows an index range scan on the new index.

### Step 3 — data-service endpoint

**Subagent:** `go-backend`.

**Prompt:**
> Implement `GET /api/v1/stations/{externalId}/board` in `services/go/data-service` exactly
> as specified in `specs/openapi/data-service.yml` (`getStationBoard`) and
> `docs/proposals/station-board.md` §2.1, §2.2 and §2.4. Follow `services/go/CLAUDE.md` and
> `services/go/data-service/CLAUDE.md`: raw SQL via `r.db.WithContext(ctx).Raw(...)`,
> positional params, and tracing spans `db.station_board.query` / `station.board`.
> Files:
> - `services/go/shared/dsmodel/model.go`: add `StationBoardResponse` and `StationBoardEntry`
>   (JSON tags per spec). Add aliases in `data-service/internal/model/model.go` and
>   `gateway/internal/client/dataservice/types.go`, matching the existing alias pattern.
> - `data-service/internal/repository/operations.go`: `QueryStationBoardCandidates(ctx, stationExtID int, now time.Time, window, lookback time.Duration)`
>   using the SQL in §2.4 verbatim (the predicate must match the Step 2 index expression).
>   Also return `max(t.updated_at)` as data_as_of.
> - `data-service/internal/service/`: extend the `Repository` interface, and add a **pure**
>   function `BucketStationBoard(rows, now, window) []StationBoardEntry` that implements the
>   §2.1 table: `at_station`, `arrival` and `departure`. A through train can yield both
>   arrival and departure. At-station stops are excluded from departure. A terminus counts
>   as at station for 5 min after arrival. Cancelled stops are never at_station. Order by
>   board time.
> - `data-service/internal/handler/`: a handler on `r.Get("/stations/{externalId}/board", …)`
>   next to the existing station routes in `handler.go` (keep static-before-param ordering).
>   It parses and validates `at`, `windowMinutes` and `lookbackMinutes` (400 on bad input),
>   returns 404 if the station does not exist (reuse `GetStationByExternalId`), and gets
>   "now" from a single helper `boardNow(r)`. If Step 0 has **not** landed, `boardNow`
>   applies the §2.2 option B shim (Warsaw wall clock expressed as UTC) with
>   `// TODO(fix/data-freshness): remove once times are stored as true UTC`.
> Tests (new; this module has none yet): `internal/service/station_board_test.go`, a
> table-driven test for `BucketStationBoard` covering origin, terminus, through train in
> both buckets, standing at platform, delayed-into-window, delayed-out-of-window,
> cancelled, and midnight crossing. Add `internal/handler` tests with a fake service for
> param validation and 404. `make data-service-test` and `make data-service-lint` must pass.
> Add swag annotations like the neighbouring handlers. Do not change other endpoints.
>
> Acceptance: `curl localhost:8083/api/v1/stations/33506/board?windowMinutes=60` returns
> spec-conformant JSON, and a p95 under 50 ms on local data after Step 2.

### Step 4 — gateway endpoint

**Subagent:** `go-backend`.

**Prompt:**
> Implement `GET /api/v1/stations/{externalId}/board` in `services/go/gateway` per
> `specs/openapi/gateway.yml` (`getStationBoard`) and `docs/proposals/station-board.md`
> §2.3 and §2.5. Follow `services/go/CLAUDE.md` and `services/go/gateway/CLAUDE.md`.
> - `internal/client/dataservice/client.go`: `GetStationBoard(ctx, externalID int, windowMinutes int) (*StationBoardResponse, error)`,
>   using the existing `doRequest`. Map 404 the same way `GetStationByExternalID` does.
> - `internal/model/gateway.go`: `StationBoardView` and `StationBoardRow` per spec.
> - `internal/service/service.go`: `GetStationBoard(ctx, externalID, window, limit)`. Call
>   `GetStationByExternalID`, `GetStationBoard` and `loadCarrierMap` (3 calls, no per-row
>   calls). Split entries into `at_station` / `arrivals` / `departures`, truncating each to
>   `limit`. Choose the planned/expected time and platform per bucket (arrival fields for
>   arrivals, departure fields otherwise). Format clocks with `trainutil.FormatClock`.
>   Compute `minutes_until` against `generated_at`, and `delay_minutes` from the relevant
>   side. Put `train_name` in `shared/trainutil` as `DisplayName(routeName, category, nationalNumber, trainNumber *string, scheduleID, orderID int) string`
>   with the fallback chain from the proposal, plus unit tests. Map status via
>   `trainutil.StatusLabel`.
> - `internal/handler/handler.go`: register `r.Get("/stations/{externalId}/board", …)`.
>   Validate `window` ∈ {30,60,120} and `limit` 1–50, and return 400 otherwise, reusing the
>   existing error helpers.
> - Do not add server-side caching and do not change `Cache-Control` handling.
> Tests: extend `internal/service/service_test.go` (fake client: bucketing, name fallback,
> carrier resolution, limit, missing-route rows) and `internal/handler/handler_test.go`
> (param validation, 404 passthrough). `make gateway-test` and `make gateway-lint` must pass.
>
> Acceptance: `curl localhost:8084/api/v1/stations/33506/board` returns spec-conformant
> JSON, and rows without a route still get a non-empty `train_name`.

### Step 5 — frontend

**Subagent:** `frontend-design`. Prerequisite: issue #11 (`src/lib` gitignored) is resolved,
or `api.ts` is force-added.

**Prompt:**
> Build the station board UI in `services/frontend` per `docs/proposals/station-board.md`
> §2.6. Follow `services/frontend/CLAUDE.md`. Gateway shapes are in
> `specs/openapi/gateway.yml` (`StationBoardView`, `StationBoardRow`).
> - `src/lib/api.ts`: add the `StationBoardView`/`StationBoardRow` interfaces and
>   `gateway.getStationBoard(id, window, limit?)` → `/api/v1/stations/{id}/board`.
>   Add a hook `useStationBoard(id, window)` in `src/lib/` (react-query,
>   `queryKey ['stationBoard', id, window]`, `staleTime 30_000`, `refetchInterval 60_000`,
>   `enabled: id != null`).
> - `src/components/StationBoard.tsx`: `variant: 'panel' | 'page'`. Header (name, city,
>   30/60/120 window selector), freshness line (`generated_at` and `data_as_of`, with an
>   amber warning if `data_as_of` is more than 30 min old), and sections "Na stacji",
>   "Przyjazdy", "Odjazdy" (tabs on narrow screens, stacked otherwise). Each row is a
>   `Link` to `/pociagi/{operation_id}` showing the time (planned, plus expected when
>   delayed), train name and number, carrier, from/to, platform/track,
>   `Badge variant={delayVariant(...)}`/`formatDelay`, a ticking "za N min" (recomputed
>   every 30 s client-side), and a cancelled style. Include loading skeletons, an error
>   card with retry (same pattern as `QueryMessage` in `MapHomeClient.tsx`), per-section
>   empty states, and a 404 state. Copy is in Polish.
> - `src/components/TrafficMapClient.tsx`: accept `selectedStationId?: number` and
>   `onStationSelect?: (s: MapStation) => void`. The marker click calls it instead of
>   `openPopup()`. Remove the `Popup`/`StationPopupContent` and keep the label tooltip.
>   The selected marker gets a larger radius and an accent colour, and the map pans to it.
>   Keep the diff minimal; `feature/train-positions` is also editing this file.
> - `src/components/MapHomeClient.tsx`: read and write `?stacja=<id>` via
>   `useSearchParams` + `router.replace`. When it is set, render
>   `<StationBoard variant="panel">` in the existing panel container (same responsive
>   classes), with a "← Przegląd sieci" back button, `Esc` to close, and focus moved to
>   the board heading. Otherwise render the existing overview unchanged. Add a
>   "Pełna tablica" link to `/stacje/{id}`. Wrap `useSearchParams` usage in `Suspense`
>   as Next 15 requires.
> - `src/app/stacje/[id]/page.tsx`: `NavShell` + `<StationBoard variant="page">`.
> `npm run build` must pass (the only gate). Manually verify with the stack running
> (`npm run dev`): click Warszawa Zachodnia on `/mapa` and the panel opens; the back
> button closes it; a row click goes to `/pociagi/{id}`; `/stacje/33506` renders; mobile
> width 375 px uses the bottom sheet with no horizontal scroll. Do not touch unrelated
> pages.
>
> Acceptance: build is green, the flows above work, and the empty, loading and error
> states have been seen (e.g. stop data-service to see the error state).

### Step 6 — Freshness (follow-up, shared; not on this branch)

**Subagent:** `data-engineering` (DAG), plus `devops-infra` if compose or Airflow config changes.

**Prompt:**
> Add intraday operations ingestion for Pociag do Predykcji so live views (station board,
> train positions) are not stuck on the 02:00 snapshot. Today `ingest_operations_daily`
> runs `0 2 * * *` (`airflow/dags/ingest_operations_daily.py:57`) and fetches operations
> once. Design and implement a DAG `ingest_operations_intraday` (for example
> `*/10 5-23 * * *` Europe/Warsaw, `max_active_runs=1`, `catchup=False`) that calls the
> collector `POST /api/v1/fetch/operations` for today's Warsaw service date with
> `force: true` (check the collector spec `specs/openapi/collector.yml` for supported
> params, and change the spec first if a parameter is missing), then
> `process_operations`. The upsert is already idempotent. First confirm the PLK API quota
> and rate limits (open question), and check that `is_pipeline_running` does not block
> parallel daily and intraday runs incorrectly. Emit `pociag.data.operations_ingested` as
> today. Add DAG and pipeline unit tests. `ruff`, `mypy` and `pytest` must pass.
>
> Acceptance: during the day `max(train_operations.updated_at)` stays under 15 min old,
> and the board's `data_as_of` reflects it.

### Step 7 — Operational: refresh schedules (no code)

Trigger `ingest_schedules_weekly` in the local stack (Airflow UI :8090) so `routes` covers
current operating dates. Today only 1 of 2,340 operations joins to a route. Then check
`SELECT count(r.id) FROM train_operations t LEFT JOIN routes r USING (schedule_id, order_id) WHERE t.operating_date = current_date`.

---

## 4. Open questions / decisions for you

1. **Raw timestamp format.** Do PLK `plannedArrival` strings carry `Z`, an offset, or
   nothing? The answer decides whether the Step 0 fix is in parsing or in PLK semantics.
   (The spec text says "UTC", but the data looks like local time.)
2. **Who owns the time contract?** I propose `fix/data-freshness` (Step 0). Is the board
   allowed to ship first with the isolated option B shim in `boardNow()`?
3. **Bucket semantics.** Should a through train appear in both Przyjazdy and Odjazdy
   (proposed, like real boards), or once only, in whichever event comes next?
4. **Default window.** 60 min (proposed) with 30/120 options? For small stations 60 min is
   often empty. Should the empty state offer "pokaż następne 3 pociągi" (requires a
   `next N` mode instead of a window)?
5. **Freshness expectation.** Is a board with morning-snapshot delays plus a staleness
   warning acceptable for v1, or should Step 6 (intraday ingestion) block release? What is
   the PLK API quota?
6. **Caching.** Is `Cache-Control: no-store` globally OK for now, or do you want per-route
   `max-age` (a middleware change)?
7. **Entry points.** Map only, or also link stations from search results and from the
   train view's stop list (`/pociagi/[id]`, owned by train-details) to `/stacje/[id]`?
8. **URL slugs.** `/stacje/[id]` and `?stacja=` (Polish, consistent with existing routes). OK?
9. **Station coordinates.** Only 93 stations are clickable. Is expanding
   `plugins/pociag_processing/data/*.json` coverage in scope somewhere?
