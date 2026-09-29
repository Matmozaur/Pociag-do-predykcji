# Proposal: Station board ("tablica stacyjna")

Status: **accepted, ready to implement** (decisions recorded in §4, 2026-09-29).
Branch: `feature/station-board`.

## The request

> I want a view for stations with data like trains currently at the station, trains about to
> arrive, and trains about to depart. I want to be able to open it by clicking on the station
> (on the map).

## TL;DR

- **The data supports a board.** `operation_stations` has planned, actual/expected times,
  delays, confirmation and cancellation flags for each stop of each train on each day. A
  prototype query for Warszawa Zachodnia returned plausible rows in about 87 ms. That time
  grows as history accumulates, so we add one index (migration `011`).
- **The two earlier data blockers are resolved** by `fix/data-freshness` (PR #28, merged into
  this branch):
  1. Operation stop times are now stored as true UTC instants (plugin parses PLK naive times
     as `Europe/Warsaw`; migration `010_operation_stations_warsaw_tz` corrected history), and
     `trainutil.FormatClock` renders in `Europe/Warsaw`. The board can compare against a plain
     `time.Now()`, with no shim.
  2. `ingest_operations_live` refreshes operations every 10 min, so delays and "at station"
     states are near-live. The board still shows `data_as_of` with the existing staleness
     warning.
- **Remaining local-environment gap:** `routes` (train name, category, platform) was last
  loaded on 2026-07-19 (#25), so most of today's operations do not join to a route. The board
  degrades gracefully (display-name fallback, platform optional).
- **Design:** a new data-service endpoint `GET /api/v1/stations/{externalId}/board` returns
  bucketed stop events (a pure Go function does the bucketing). A new gateway endpoint
  `GET /api/v1/stations/{externalId}/board` shapes them into `at_station` / `arrivals` /
  `departures`, **the next 10 trains per section by default**. On the frontend, clicking a
  station on the map opens a **board panel** in the existing map side panel or bottom sheet,
  with state in the URL (`/mapa?station=33506`). The same `StationBoard` component backs a
  full page at `/stations/[id]`, which is also linked from station search and from the train
  view's stop list. Each row links to `/pociagi/{operation_id}`.

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
- The map lives inside `MapHomeClient` (`services/frontend/src/components/MapHomeClient.tsx`).
  Its "Centrum operacyjne" panel is a left side card on desktop and a bottom sheet on mobile.
  That panel is the natural place for a station board.
- `/mapa` is the home page (`services/frontend/src/app/page.tsx` redirects to `/mapa`;
  `app/mapa/page.tsx` dynamically imports `MapHomeClient` with `ssr:false`).
- **Only 93 of 3,319 stations have coordinates** (#26, deferred), so only 93 stations are
  clickable on the map. The board itself works for any station `external_id`, which is why the
  search and train-view entry points matter.

### 1.2 Existing station, operations and train endpoints

| Layer | Endpoint | Where |
|---|---|---|
| gateway | `GET /api/v1/search/stations` | `specs/openapi/gateway.yml`, `handler.go` |
| gateway | `GET /api/v1/map/stations` | `gateway.yml`, `handler.go`, `service.go` |
| gateway | `GET /api/v1/trains/live?stations=` | `service.go` (status `P` only, **N+1** `GetOperationByID` per row, #18) |
| gateway | `GET /api/v1/trains/{operationId}` | `service.go` (the train view board rows link to; `TrainStopView` already carries `station_external_id`) |
| data-service | `GET /api/v1/stations`, `/stations/{externalId}` | `data-service.yml`, `handler.go` |
| data-service | `GET /api/v1/operations?stationExternalIds=&date=` | `repository/operations.go`. Filters operations that *touch* a station, but returns no per-stop times |
| data-service | `GET /api/v1/operations/{id}` | `operations.go`. Per-stop times for **one** train |

**No existing endpoint answers "which trains are at, or about to arrive at or depart from,
station X around time T".** `trains/live?stations=` is the closest, but it only covers status
`P`, has no per-station times, and costs N+1 calls.

The frontend already has `/pociagi/[id]` (train detail by `operation_id`) at
`services/frontend/src/app/pociagi/[id]/page.tsx`.

### 1.3 Curated tables

(The task brief says `route_stops`; the real table is `route_stations`.)

| Table | Board-relevant columns | Migration |
|---|---|---|
| `train_operations` | `id, schedule_id, order_id, operating_date, train_status (S/P/C/X/Q), updated_at` | `003_operations.up.sql` |
| `operation_stations` | `station_external_id, planned_sequence_number, actual_sequence_number, planned_arrival/departure, actual_arrival/departure (timestamptz, true UTC since 010), arrival/departure_delay_minutes, is_confirmed, is_cancelled` | `003`, `010` |
| `routes` | `name, carrier_code, national_number, commercial_category_symbol`, key `(schedule_id, order_id)` | `002_schedules.up.sql` |
| `route_stations` | `arrival/departure_platform`, `arrival/departure_track`, `arrival/departure_train_number`, key `(route_id, order_number)` | `002_schedules.up.sql` |
| `stations` | `external_id, name, city, latitude, longitude` | `001`, `009` |
| `carriers` | `code, name` | `001` |

Validated joins:

- `operation_stations` → `train_operations` on `train_operation_id`.
- `train_operations` → `routes` on `(schedule_id, order_id)` (the same join `operations.go` uses).
- `operation_stations.planned_sequence_number = route_stations.order_number` (same `route_id`).
- Origin and destination are the first and last `actual_sequence_number` of the operation.

Existing indexes: `idx_operation_stations_station (station_external_id)` only. Nothing on time.

### 1.4 Data quality (local DB)

| Fact | Impact on board |
|---|---|
| ~28 k Warszawa Zachodnia stop rows across all history | Station-only index scans all of them → add the §2.4 index |
| `actual_arrival`/`actual_departure` are filled for unconfirmed future stops too: PLK puts the *expected* time there (`actual = planned + delay`) | Use them as expected times |
| `actual_arrival` is present where `planned_arrival` is NULL (origin stops) | Derive "has an arrival" from `planned_arrival`, not `actual_arrival` |
| `routes` last loaded 2026-07-19 (#25) | Name, category, carrier and platform are often missing locally. Needs a fallback |
| `routes.name` non-null for only ~36 % of routes (regional trains have no name) | Display fallback: `category + national_number` |
| `route_stations.departure_platform` non-null ~85 % | Platform shown where routes are fresh |

### 1.5 Time and freshness (resolved by `fix/data-freshness`, #28)

- Stop times are true UTC instants; `_parse_plk_local_timestamp`
  (`airflow/plugins/pociag_processing/repository.py`) reads naive PLK times as `Europe/Warsaw`.
- `trainutil.FormatClock` (`services/go/shared/trainutil/util.go`) formats in `Europe/Warsaw`;
  the gateway imports `time/tzdata` in `cmd/main.go`.
- The gateway computes "today" with `warsawToday` (`gateway/internal/service/service.go`).
  data-service still uses the process-local day when `date` is omitted (#29). The board does
  not depend on that, because it filters by instant, not by operating date.
- `ingest_operations_live` runs `*/10 * * * *`. The frontend has a `DataFreshness` component
  (`services/frontend/src/components/DataFreshness.tsx`, stale after 20 min) that the board
  reuses.

### 1.6 Gateway caching today

- Only a global `Cache-Control` header from env `CACHE_CONTROL` (default `no-store`), set via
  `middleware.CacheHeaders`. No in-process response cache. This stays as it is for v1 (§4, Q6).

---

## 2. Design

### 2.1 Definitions (the board semantics)

For station `S`, reference instant `now`, per-section limit `N` (default **10**), and horizon
`H` (how far ahead to look for the next `N` trains; default 720 min):

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
| `arrival` | `has_arr` and `now < exp_arr <= now + H` |
| `departure` | `has_dep` and `now < exp_dep <= now + H`, and **not** in `at_station` |

Each bucket is ordered by its board time (`exp_arr` for arrivals, `exp_dep` for departures and
at-station) and truncated to the first `N`.

- **A through train appears in both Przyjazdy and Odjazdy** (decided, Q3), as on a real station
  board. A train standing at the platform appears only in `at_station`, which shows its
  departure time.
- Cancelled stops (`is_cancelled`) stay in arrivals and departures, flagged, and never go in
  `at_station`. They count toward `N`.
- Lookback: SQL only reads stops with `COALESCE(planned_departure, planned_arrival)` in
  `[now - 6h, now + H]`. The 6 h lower bound covers delays up to 6 h. Filtering by time
  instead of `operating_date` handles trains that run past midnight.
- Count-based, not window-based (Q4): small stations still show their next 10 trains even if
  they are hours away. `H = 720` bounds the scan; a section can have fewer than `N` rows if
  fewer trains are scheduled in the next 12 h.

### 2.2 "now"

Stored times are true UTC (§1.5), so the board uses the real instant. data-service takes `now`
from **one helper** `boardNow(r *http.Request) (time.Time, error)`: the optional `at` query
param (RFC 3339, for tests and debugging) or `time.Now().UTC()`. No timezone shim.

### 2.3 API (contract-first)

#### data-service: `GET /api/v1/stations/{externalId}/board`

Query: `at` (RFC 3339 date-time, optional, default now), `limit` (int per bucket, 1–50,
default 10), `horizonMinutes` (int, 30–1440, default 720), `lookbackMinutes` (int, 0–720,
default 360). Tag `operations`.

`200` → `StationBoardResponse`:

```yaml
StationBoardResponse:
  required: [station_external_id, at, limit, horizon_minutes, entries]
  properties:
    station_external_id: integer
    at:               date-time       # the reference instant actually used
    limit:            integer
    horizon_minutes:  integer
    data_as_of:       date-time       # max(train_operations.updated_at) over returned ops (nullable)
    entries: [StationBoardEntry]      # bucketed, truncated per bucket, ordered by board time
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
    arrival_platform, arrival_track, departure_platform, departure_track   # nullable
    planned_arrival, planned_departure, expected_arrival, expected_departure   # date-time, nullable
    arrival_delay_minutes, departure_delay_minutes                          # nullable
    is_confirmed, is_cancelled
    origin_station_external_id, origin_station_name
    destination_station_external_id, destination_station_name
```

`404` if the station `externalId` does not exist. `400` for bad params.

One entry per (stop, bucket), so a through train can yield an `arrival` and a `departure`
entry. The data-service does bucketing and truncation, so the gateway stays a pure shaper and
the rules are tested in one place.

#### gateway: `GET /api/v1/stations/{externalId}/board`

Query: `limit` (per section, 1–50, default **10**). Tag `stations` (add it to `tags:`).

`200` → `StationBoardView`:

```yaml
StationBoardView:
  required: [station, generated_at, at_station, arrivals, departures]
  properties:
    station: { external_id, name, city?, latitude?, longitude? }
    generated_at:   date-time
    data_as_of:     date-time          # fed to the existing DataFreshness component
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
    expected_at:   date-time           # instant behind expected_time, so the UI can tick "in N min"
    delay_minutes: integer
    platform, track
    status: enum [not_started, in_progress, completed, cancelled, partial_cancelled]
    is_cancelled, is_confirmed
```

(`minutes_until` from the first draft is replaced by `expected_at`: the client computes the
countdown itself, so it does not drift between refetches.)

Gateway fan-out: `GetStationByExternalID` + new `GetStationBoard` + `ListCarriers` (the
existing `loadCarrierMap`). 3 calls in total, no N+1.

### 2.4 SQL (data-service repository)

Placeholders: `$1` station, `$2` now, `$3` horizon interval, `$4` lookback interval.

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

- Go then buckets the rows (§2.1) and truncates each bucket to `limit`.
- A 12 h horizon at a busy station is a few hundred candidate rows, well within budget once
  indexed. If profiling shows otherwise, lower the default horizon rather than adding SQL
  `LIMIT` (bucketing needs all candidates).
- The lateral lookups use the existing `uq_operation_station (train_operation_id, actual_sequence_number)`.
- **Index (migration `011_operation_stations_station_time`; `010` is taken by the timezone fix):**
  ```sql
  CREATE INDEX IF NOT EXISTS idx_operation_stations_station_planned_time
      ON operation_stations (station_external_id, (COALESCE(planned_departure, planned_arrival)));
  ```
  The predicate in the SQL must use exactly this expression. Down: `DROP INDEX IF EXISTS`.
  Leave `idx_operation_stations_station` in place; `QueryOperations` still uses it.

### 2.5 Gateway caching

**v1: no server-side cache, global `Cache-Control: no-store` unchanged** (decided, Q6). The
client refreshes every 60 s and data changes every 10 min, so load is tiny. A 30 s in-process
TTL cache in `internal/service` is the fallback if load ever matters.

### 2.6 Frontend UX

URL slugs are **English** (decided, Q8): `/stations/[id]` and `?station=`. Existing routes
(`/mapa`, `/pociagi`, …) stay as they are. UI copy stays Polish.

Entry points (decided, Q7: map **and** other places):

1. **Map.** Clicking a `CircleMarker` calls `onStationSelect(station)` instead of
   `openPopup()` (`TrafficMapClient.tsx:152-154`). Drop the popup and keep the permanent label
   tooltip. Highlight the selected marker (larger radius, accent colour) and pan to it.
   `MapHomeClient` keeps `?station=<external_id>` in the URL (`useSearchParams` +
   `router.replace`). When it is set, the side panel or bottom sheet shows
   `<StationBoard stationId=… variant="panel" />` with a back button ("← Przegląd sieci") and a
   "Pełna tablica" link to `/stations/{id}`. Otherwise the panel shows the existing overview.
   `Esc` closes it.
2. **Search.** On `/wyszukaj`, once a station is chosen in the "Skąd"/"Dokąd"
   `StationAutocomplete`, show a small "Tablica stacyjna →" link to `/stations/{external_id}`
   next to the field. Autocomplete selection behaviour is unchanged.
3. **Train view.** In `/pociagi/[id]`'s `StopTimeline`, the station name becomes a `Link` to
   `/stations/{station_external_id}` when the id is present (`TrainStopView` already has it).
   This file is also the target of `feature/train-details`, so keep the diff to that one
   element.

`/stations/[id]` page: `NavShell` + `<StationBoard variant="page" />`.

`StationBoard` contents:

- Header: station name and city.
- Freshness: the existing `<DataFreshness lastUpdated={data_as_of} />` (no new warning logic).
- Three sections: **Na stacji**, **Przyjazdy**, **Odjazdy**. Tabs on mobile, stacked on
  desktop and page. Each shows up to 10 trains; a "Pokaż więcej" button raises `limit`
  (10 → 20 → 50).
- Row: time (planned, with the expected time shown and the planned one struck through when
  delayed), train name and number, a carrier badge, "z {origin}" for arrivals or
  "do {destination}" for departures, platform/track, a delay `Badge` (`delayVariant` /
  `formatDelay`), an "za N min" label computed from `expected_at` and refreshed every 30 s,
  and a cancelled style. The whole row is a `Link` to `/pociagi/{operation_id}`, following the
  `TrainRow` pattern in `MapHomeClient.tsx`.
- States: skeleton rows while loading; an error card with "Spróbuj ponownie" (the existing
  `QueryMessage` pattern); a per-section empty state ("Brak przyjazdów w najbliższych 12 h");
  and a 404 "Nie znaleziono stacji".
- Data: `useStationBoard(id, limit)` react-query hook in `src/lib/` with
  `queryKey: ['stationBoard', id, limit]`, `staleTime: 30_000`, `refetchInterval: 60_000`,
  `refetchIntervalInBackground: false`, `enabled: id != null`.
- Accessibility: the panel is a `section` with `aria-labelledby`, focus moves to its heading
  when it opens, and `Esc` closes it.

⚠ `services/frontend/src/lib/` is still git-ignored (issue **#11**, open). `api.ts` and the
hook cannot be committed until #11 is fixed, or unless the files are force-added. Step 5
depends on this.

### 2.7 Shared pieces and sibling branches

| Shared piece | Status | Used by |
|---|---|---|
| Time contract (true-UTC storage, Warsaw `FormatClock`, `warsawToday`) | **Done** in `fix/data-freshness` (#28) | board, train-positions, train-details |
| Intraday operations ingestion (`ingest_operations_live`, every 10 min) | **Done** in #28 | board delays, train positions, live list |
| "Expected time" rule `COALESCE(actual, planned + delay, planned)` and "has arrival = planned_arrival not null" | Defined here (§2.1). train-positions should reuse the same expressions | board, train-positions |
| Train display name fallback | Board adds `trainutil.DisplayName(...)` in `shared` | board, live trains, train-details |
| `/pociagi/[id]` train view | Owned by **feature/train-details**. The board links to it by `operation_id` and adds one `Link` in `StopTimeline` (§2.6) | coordinate on merge order |
| Map components `TrafficMapClient.tsx` / `MapHomeClient.tsx` | Board changes the marker click and the panel. **train-positions** will add a train layer | Keep the board's diff to the click handler, a selected-marker prop, and a panel switch. train-positions adds a separate `<Pane>` |

### 2.8 Out of scope (v1)

Platform-change detection, connection info (`route_connections`), disruptions for the station
(`queryDisruptions?stationExternalIds=` exists and could become a banner later), historical
boards (the `at` param allows it, but there is no UI), push updates, station coordinate
coverage (#26, later), refreshing schedules (#25, operational).

---

## 3. Implementation steps

Each step is one small PR-sized commit on `feature/station-board`. The prompts are
self-contained. Order: 1 → 2 → 3 → 4 → 5 (Step 2 can run in parallel with 1).

### Step 1 — Contract: spec changes

**Subagent:** `go-backend`.

**Prompt:**
> Contract-first spec change for a station board feature in Pociag do Predykcji. Read
> `docs/proposals/station-board.md` §2.1 and §2.3 and follow them exactly.
> (1) In `specs/openapi/data-service.yml`, add `GET /api/v1/stations/{externalId}/board`
> (operationId `getStationBoard`, tag `operations`, params `at` date-time, `limit` 1–50
> default 10, `horizonMinutes` 30–1440 default 720, `lookbackMinutes` 0–720 default 360, and
> responses 200 `StationBoardResponse`, 400, 404). Add schemas `StationBoardResponse` and
> `StationBoardEntry` with `additionalProperties: false`, matching the proposal field list.
> Timestamps are `format: date-time` and nullable fields are optional.
> (2) In `specs/openapi/gateway.yml`, add `GET /api/v1/stations/{externalId}/board`
> (operationId `getStationBoard`, param `limit` 1–50 default 10, responses 200
> `StationBoardView`, 400, 404). Add schemas `StationBoardView` and `StationBoardRow`
> (including `expected_at` date-time; no `minutes_until`, no window), and a `stations` tag in
> the `tags:` list.
> Bump `info.version` minor in both files. Only edit these two files. Validate that both
> parse (for example `npx @redocly/cli lint` or a YAML load). Do not edit code.
>
> Acceptance: both specs lint and parse, and no other file is changed.

### Step 2 — Index migration

**Subagent:** `data-engineering`.

**Prompt:**
> Add migration `db/migrations/011_operation_stations_station_time.up.sql` / `.down.sql`
> (`010` is taken). Up:
> `CREATE INDEX IF NOT EXISTS idx_operation_stations_station_planned_time ON operation_stations (station_external_id, (COALESCE(planned_departure, planned_arrival)));`
> in `BEGIN/COMMIT`. Down: `DROP INDEX IF EXISTS idx_operation_stations_station_planned_time;`.
> Follow `db/migrations/CLAUDE.md`. Apply with `make db-migrate-up`, roll back with
> `make db-migrate-down`, then apply again. Verify with `EXPLAIN ANALYZE` that
> `WHERE station_external_id = 33506 AND COALESCE(planned_departure, planned_arrival) BETWEEN now()-interval '6 hours' AND now()+interval '12 hours'`
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
> Stored times are true UTC instants; do not add any timezone shim.
> Files:
> - `services/go/shared/dsmodel/model.go`: add `StationBoardResponse` and `StationBoardEntry`
>   (JSON tags per spec). Add aliases in `data-service/internal/model/model.go` and
>   `gateway/internal/client/dataservice/types.go`, matching the existing alias pattern.
> - `data-service/internal/repository/operations.go`: `QueryStationBoardCandidates(ctx, stationExtID int, now time.Time, horizon, lookback time.Duration)`
>   using the SQL in §2.4 verbatim (the predicate must match the Step 2 index expression).
>   Also return `max(t.updated_at)` as data_as_of.
> - `data-service/internal/service/`: extend the `Repository` interface, and add a **pure**
>   function `BucketStationBoard(rows, now, horizon, limit) []StationBoardEntry` that
>   implements the §2.1 table: `at_station`, `arrival` and `departure`. A through train
>   yields both arrival and departure. At-station stops are excluded from departure. A
>   terminus counts as at station for 5 min after arrival. Cancelled stops are never
>   at_station. Each bucket is ordered by its board time and truncated to `limit`.
> - `data-service/internal/handler/`: a handler on `r.Get("/stations/{externalId}/board", …)`
>   next to the existing station routes in `handler.go` (keep static-before-param ordering).
>   It parses and validates `at`, `limit`, `horizonMinutes` and `lookbackMinutes` (400 on bad
>   input), returns 404 if the station does not exist (reuse `GetStationByExternalId`), and
>   gets "now" from a single helper `boardNow(r)` (`at` or `time.Now().UTC()`).
> Tests (this module has none yet, #23): `internal/service/station_board_test.go`, a
> table-driven test for `BucketStationBoard` covering origin, terminus, through train in
> both buckets, standing at platform, delayed-into-horizon, delayed-out-of-horizon,
> cancelled, midnight crossing, and truncation to `limit`. Add `internal/handler` tests with
> a fake service for param validation and 404. `make data-service-test` and
> `make data-service-lint` must pass. Add swag annotations like the neighbouring handlers.
> Do not change other endpoints.
>
> Acceptance: `curl 'localhost:8083/api/v1/stations/33506/board?limit=10'` returns
> spec-conformant JSON, with p95 under 50 ms on local data after Step 2.

### Step 4 — gateway endpoint

**Subagent:** `go-backend`.

**Prompt:**
> Implement `GET /api/v1/stations/{externalId}/board` in `services/go/gateway` per
> `specs/openapi/gateway.yml` (`getStationBoard`) and `docs/proposals/station-board.md`
> §2.3 and §2.5. Follow `services/go/CLAUDE.md` and `services/go/gateway/CLAUDE.md`.
> - `internal/client/dataservice/client.go`: `GetStationBoard(ctx, externalID int, limit int) (*StationBoardResponse, error)`,
>   using the existing `doRequest`. Map 404 the same way `GetStationByExternalID` does.
> - `internal/model/gateway.go`: `StationBoardView` and `StationBoardRow` per spec.
> - `internal/service/service.go`: `GetStationBoard(ctx, externalID, limit)`. Call
>   `GetStationByExternalID`, `GetStationBoard` and `loadCarrierMap` (3 calls, no per-row
>   calls). Split entries into `at_station` / `arrivals` / `departures` (data-service already
>   truncated them). Choose the planned/expected time, `expected_at`, delay and
>   platform/track per bucket (arrival fields for arrivals, departure fields otherwise).
>   Format clocks with `trainutil.FormatClock`. Put `train_name` in `shared/trainutil` as
>   `DisplayName(routeName, category, nationalNumber, trainNumber *string, scheduleID, orderID int) string`
>   with the fallback chain from the proposal, plus unit tests. Map status via
>   `trainutil.StatusLabel`.
> - `internal/handler/handler.go`: register `r.Get("/stations/{externalId}/board", …)`.
>   Validate `limit` 1–50 (default 10) and return 400 otherwise, reusing the existing error
>   helpers.
> - Do not add server-side caching and do not change `Cache-Control` handling.
> Tests: extend `internal/service/service_test.go` (fake client: bucket split, name fallback,
> carrier resolution, missing-route rows, arrival vs departure field selection) and
> `internal/handler/handler_test.go` (param validation, 404 passthrough).
> `make gateway-test` and `make gateway-lint` must pass.
>
> Acceptance: `curl localhost:8084/api/v1/stations/33506/board` returns spec-conformant
> JSON with up to 10 rows per section, and rows without a route still get a non-empty
> `train_name`.

### Step 5 — frontend

**Subagent:** `frontend-design`. Prerequisite: issue #11 (`src/lib` git-ignored) is resolved,
or `api.ts` and the new hook are force-added.

**Prompt:**
> Build the station board UI in `services/frontend` per `docs/proposals/station-board.md`
> §2.6. Follow `services/frontend/CLAUDE.md`. Gateway shapes are in
> `specs/openapi/gateway.yml` (`StationBoardView`, `StationBoardRow`). URL slugs are
> English (`/stations/[id]`, `?station=`); UI copy is Polish.
> - `src/lib/api.ts`: add the `StationBoardView`/`StationBoardRow` interfaces and
>   `gateway.getStationBoard(id, limit?)` → `/api/v1/stations/{id}/board`. Add a hook
>   `useStationBoard(id, limit)` in `src/lib/` (react-query,
>   `queryKey ['stationBoard', id, limit]`, `staleTime 30_000`, `refetchInterval 60_000`,
>   `refetchIntervalInBackground: false`, `enabled: id != null`).
> - `src/components/StationBoard.tsx`: `variant: 'panel' | 'page'`. Header (name, city),
>   the existing `DataFreshness` component fed with `data_as_of`, and sections "Na stacji",
>   "Przyjazdy", "Odjazdy" (tabs on narrow screens, stacked otherwise), 10 rows by default
>   with a "Pokaż więcej" button stepping `limit` 10 → 20 → 50. Each row is a `Link` to
>   `/pociagi/{operation_id}` showing the time (planned, plus expected when delayed), train
>   name and number, carrier, from/to, platform/track,
>   `Badge variant={delayVariant(...)}`/`formatDelay`, an "za N min" label computed from
>   `expected_at` and refreshed every 30 s, and a cancelled style. Include loading
>   skeletons, an error card with retry (same pattern as `QueryMessage` in
>   `MapHomeClient.tsx`), per-section empty states, and a 404 state.
> - `src/components/TrafficMapClient.tsx`: accept `selectedStationId?: number` and
>   `onStationSelect?: (s: MapStation) => void`. The marker click calls it instead of
>   `openPopup()`. Remove the `Popup`/`StationPopupContent` and keep the label tooltip.
>   The selected marker gets a larger radius and an accent colour, and the map pans to it.
>   Keep the diff minimal; `feature/train-positions` also edits this file.
> - `src/components/MapHomeClient.tsx`: read and write `?station=<id>` via
>   `useSearchParams` + `router.replace`. When it is set, render
>   `<StationBoard variant="panel">` in the existing panel container (same responsive
>   classes), with a "← Przegląd sieci" back button, `Esc` to close, focus moved to the
>   board heading, and a "Pełna tablica" link to `/stations/{id}`. Otherwise render the
>   existing overview unchanged. Wrap `useSearchParams` usage in `Suspense` as Next 15
>   requires.
> - `src/app/stations/[id]/page.tsx`: `NavShell` + `<StationBoard variant="page">`.
> - `src/app/wyszukaj/page.tsx`: when a station is selected in either `StationAutocomplete`,
>   show a small "Tablica stacyjna →" link to `/stations/{external_id}` next to it. Do not
>   change selection or search behaviour.
> - `src/app/pociagi/[id]/page.tsx`: in `StopTimeline`, wrap the station name in a `Link` to
>   `/stations/{station_external_id}` when the id is present. Change nothing else in that
>   file (`feature/train-details` owns it).
> `npm run build` must pass (the only gate). Manually verify with the stack running
> (`npm run dev`): click Warszawa Zachodnia on `/mapa` and the panel opens; the back button
> closes it; a row click goes to `/pociagi/{id}`; `/stations/33506` renders; the search and
> train-view links reach the board; mobile width 375 px uses the bottom sheet with no
> horizontal scroll. Do not touch unrelated pages.
>
> Acceptance: build is green, the flows above work, and the empty, loading and error states
> have been seen (e.g. stop data-service to see the error state).

### Operational (no code): refresh schedules

Trigger `ingest_schedules_weekly` in the local stack (Airflow UI :8090) so `routes` covers
current operating dates (#25). Then check
`SELECT count(r.id) FROM train_operations t LEFT JOIN routes r USING (schedule_id, order_id) WHERE t.operating_date = current_date`.

---

## 4. Decisions (2026-09-29)

| # | Question | Decision |
|---|---|---|
| 1–2 | Raw timestamp format / who owns the time contract | Resolved by `fix/data-freshness` (#28): naive PLK times parsed as `Europe/Warsaw`, history migrated (`010`), Warsaw `FormatClock`. The old Step 0 and the `boardNow` shim are dropped |
| 3 | Through train in both Przyjazdy and Odjazdy? | **Yes** |
| 4 | Default window | **Next 10 trains per section** (count-based, 12 h horizon), "Pokaż więcej" for up to 50. The 30/60/120 window selector is dropped |
| 5 | Must intraday freshness block release? | **Not a blocker.** In any case `ingest_operations_live` (every 10 min) already exists; the board shows `DataFreshness` |
| 6 | Caching | Global `Cache-Control: no-store` is **OK** for v1 |
| 7 | Entry points | **Both**: map click, plus links from station search (`/wyszukaj`) and the train view's stop list |
| 8 | URL slugs | **English**: `/stations/[id]`, `?station=` |
| 9 | Station coordinates (only 93 clickable) | **Later**, tracked in #26 |
