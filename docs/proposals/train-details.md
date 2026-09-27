# Proposal: Train details view (current position, next stations, ETAs)

Status: **draft for review** · Branch: `feature/train-details` · Date: 2026-09-27

> "I want a view for trains with their current positions and their next stations, along with
> estimated arrival times. I want to be able to open it by clicking on the train (on the map)."

## TL;DR

- About 70% of this already exists. The gateway has `GET /api/v1/trains/{operationId}`, and
  `/pociagi/[id]` renders a stop timeline. What's missing: **ETAs, a "where is it now" state,
  a map entry point, and trustworthy time data**.
- **The main risks are in the data, not the UI.** Checked against the local DB:
  1. Operations are ingested **once a day at 02:00**, so any "current position" can be up to
     24 h old.
  2. `operation_stations` timestamps look like **Polish wall-clock time stored as if it were UTC**
     (they are off by the CET/CEST offset).
  3. PLK already fills `actual_*` for *unconfirmed* stops with a **forecast** (planned + delay).
     That gives ETAs almost for free.
- Recommendation: make **additive** changes to the existing `TrainDetailView` contract. Put the
  ETA/progress logic in a **pure `shared/trainutil` package** that `feature/train-positions`
  reuses. In the frontend, a **map side drawer** (`/mapa?train=<operationId>`) shares a timeline
  component with the full page `/pociagi/<operationId>`.
- Shared train identifier: **`operation_id`** (`train_operations.id`), the same value that
  `/api/v1/trains/live` and the existing `/pociagi/[id]` links already use.

---

## 1. Current state (evidence)

### 1.1 Map
- The map draws **stations only**, as `CircleMarker`s, plus OpenRailwayMap tiles. There are no
  train markers and no route lines: `services/frontend/src/components/TrafficMapClient.tsx:145-170`.
- The home overlay says so: *"Dane operacyjne są prezentowane na liście, bez przybliżania pozycji
  pociągów"* (`services/frontend/src/components/MapHomeClient.tsx:296`).
- Live-train rows in the overlay already link to `/pociagi/{operation_id}`
  (`MapHomeClient.tsx:222-236`). Clickable train markers are the job of `feature/train-positions`.

### 1.2 Existing train detail path (end to end)
| Layer | Evidence |
|---|---|
| Gateway spec | `specs/openapi/gateway.yml:238-263` (`GET /api/v1/trains/{operationId}`), schemas `TrainStopView` / `TrainDetailView` at `gateway.yml:655-718` |
| Gateway route | `services/go/gateway/internal/handler/handler.go:46` |
| Gateway service | `services/go/gateway/internal/service/service.go:374-430` (`GetTrainDetail`): copies stops through and formats times as `HH:MM` |
| Current/next logic | `service.go:811-851` (`currentAndNextStations`), used only by `/trains/live`: last confirmed stop is "current", first non-cancelled stop after it is "next" |
| Clock formatting | `services/go/shared/trainutil/util.go:51-57` `FormatClock`: `ts.Format("15:04")` in **the `time.Time`'s own location** (UTC from pgx) |
| Data-service spec | `specs/openapi/data-service.yml:269-290`, `OperationDetail` / `OperationStation` at `data-service.yml:865-930` |
| Data-service repo | `services/go/data-service/internal/repository/operations.go:156-297`: joins `routes` on `(schedule_id, order_id)` for name and carrier only; no `route_id`, no freshness timestamp |
| Frontend page | `services/frontend/src/app/pociagi/[id]/page.tsx:17-77` (`StopTimeline`), `:82-88` (query with `refetchInterval: 60_000`) |
| Frontend client | `services/frontend/src/lib/api.ts` `gateway.getTrainDetail` (**note:** `src/lib/` is untracked because of issue #11, so a fresh worktree has no `src/lib`) |

### 1.3 Curated tables (`db/migrations`)
- `train_operations` has a unique key on `(schedule_id, order_id, operating_date)` and a surrogate
  `id` (`003_operations.up.sql:100-111`). The upsert is `ON CONFLICT ... DO UPDATE`, so **`id` stays
  the same across re-ingestion**. That makes it a good shared identifier.
- `operation_stations` holds planned, actual and delay per stop, plus `is_confirmed` and
  `is_cancelled` (`003_operations.up.sql:118-136`).
- `routes`, `route_stations` and `route_operating_dates` hold the timetable
  (`002_schedules.up.sql:8-66`). Root `CLAUDE.md` calls the stops table `route_stops`; its real
  name is `route_stations`.
- `disruption_affected_routes` has `(schedule_id, order_id, operating_date, station_ext_id,
  sequence_number)` (`004_disruptions.up.sql:167-178`). That is enough to link a disruption to a
  train run.
- `stations.latitude/longitude` (`009_station_coordinates.up.sql`).

### 1.4 Ingestion freshness and time handling
- Operations DAG: `schedule="0 2 * * *"` (`airflow/dags/ingest_operations_daily.py:57`).
  `capture_date_today()` uses `date.today()` in the worker's timezone (`:51-52`). The collector uses
  `time.Now().UTC()` for the capture date (`services/go/collector/internal/service/service.go:190-191`).
- The collector calls PLK `/api/v1/operations` with `withPlanned=true`, so we get planned times
  and delays (`services/go/collector/internal/plk/client.go:77-83`). PLK returns runs for several
  operating dates in one snapshot.
- The plugin parses timestamps with a bare `datetime.fromisoformat(value)`
  (`airflow/plugins/pociag_processing/repository.py:39-42`, used at `:632-637`). A naive string
  would be stored in the Postgres session timezone, which is **UTC**.
- The gateway computes "today" as a **UTC** date (`gateway/internal/service/service.go:326, 439, 541`).
- The gateway sends `Cache-Control: no-store` globally (`gateway/internal/config/config.go:43`,
  `gateway/cmd/main.go:77`). The frontend caches for `revalidate: 30`.

## 2. Data validation (local DB, read-only SQL, 2026-09-27 ~15:20 UTC)

| Finding | Evidence | Impact |
|---|---|---|
| **Operations are a single morning snapshot.** Last successful operations run started 06:11 UTC. Every `train_operations.updated_at` for today is ≤ 06:13 UTC. | `ingestion_runs`, `train_operations.updated_at` | By afternoon, "current position" is 9 h old. Needs intraday refresh (step 2). |
| **Timestamps are local wall-clock stored as UTC.** 9,666 of 18,422 *confirmed* stops today have `actual_*` **later than the moment they were ingested**, by up to **2 h 12 min**. That is the CEST offset. Example: a Białystok→Olsztyn run is "confirmed" at Wielbark `07:31+00`, but it was ingested at `06:11+00`. | `operation_stations` vs `train_operations.updated_at` | Any `now`-based logic (position, "overdue", countdown) is off by 1–2 h. `HH:MM` display is correct only by accident: UTC formatting of a mislabeled local time. |
| **PLK forecasts unconfirmed stops.** 24,423 unconfirmed stops today, all with `actual_*` set. 23,247 of 23,502 unconfirmed rows that have both planned and actual arrival satisfy `actual_arrival = planned_arrival + arrival_delay_minutes`. Delay propagates along the run (e.g. a run 30–32 min late keeps `+31` on its upcoming stops). | `operation_stations` | ETA ≈ PLK `actual_*` for unconfirmed stops. No model needed for v1. |
| **Operations only list some stops.** For example, sequences 1, 2, 6, 12, 14, … | sample run | The timeline shows what PLK reports. The full timetable is in `route_stations`, but only if the route resolves (next row). |
| **Route join is broken in the local DB.** 1 of 2,340 of today's operations matches a `routes` row. Schedules were last ingested on 2026-07-19, and `route_operating_dates` runs to 2026-08-02. When schedules were fresh (Jul 19 – Aug 2), 80% matched. | `routes`, `ingestion_runs` | `train_name` / carrier are empty for almost every current train. The UI needs a fallback name. Schedules must be fresh (environment issue). |
| **Station coordinates are sparse.** 93 of 3,319 stations have lat/lon. | `stations` | Route highlight and positions on the map will be coarse (major stations only). |
| **Disruptions have not updated since 2026-07-23.** Today's run fetched 176 and upserted 0. All `date_from/date_to` are NULL. | `disruptions`, `ingestion_runs` | "Disruptions affecting this train" can't be delivered on real data yet. Deferred (step 8). |
| Stale "in progress" runs. 206/211/223 runs from 1–3 days ago still show `P`. | `train_operations` | The view must not show an old run as "live". Owned by `fix/data-freshness`. |

## 3. Design

### 3.1 Shared train identifier
**`operation_id`** = `train_operations.id` (int64).
- It is already the key of `/api/v1/trains/{operationId}`, `LiveTrainSummary.operation_id` and the
  `/pociagi/[id]` route.
- It stays stable across re-ingestion (upsert keeps the row), and one value identifies exactly one
  run on one operating date.
- `feature/train-positions` markers **must** carry `operation_id`. `feature/station-board` rows
  **must** carry `operation_id`, which they can get from the operations join; station boards are
  built from `operation_stations`.
- Rejected alternative: the natural key `(schedule_id, order_id, operating_date)` in the URL. It is
  stable across DB rebuilds, but it is verbose and nothing in the frontend uses it. Revisit if
  shareable links must survive a DB rebuild (open question Q5).

### 3.2 API: extend the existing endpoint (additive, contract-first)
**Recommended: add fields to `GET /api/v1/trains/{operationId}`.** The alternative is a new
`/trains/{id}/progress` endpoint. The existing page already consumes this endpoint, one request
per drawer keeps things simple, and additive optional fields don't break anyone.

New optional fields on `TrainDetailView` (gateway):

| Field | Type | Meaning |
|---|---|---|
| `route_id` | int64, nullable | Link to `/rozklad/{route_id}`. Null when the route didn't resolve. |
| `train_number` | string | `routes.national_number`, used for display and as a fallback name |
| `commercial_category` | string | `routes.commercial_category_symbol` |
| `origin`, `destination` | string | First and last non-cancelled stop names |
| `current_delay_minutes` | int | Delay at the last confirmed stop (same rule as `/trains/live`) |
| `progress` | object | `{ state, last_stop_sequence?, next_stop_sequence?, basis }`. `state` ∈ `not_started, at_station, between_stations, finished, cancelled, unknown`. `basis` ∈ `confirmed, estimated` (see 3.3). |
| `data_as_of` | date-time | `train_operations.updated_at`, i.e. when PLK data for this run was last ingested |
| `generated_at` | date-time | Server "now" used for the `progress` calculation |

New optional fields on `TrainStopView`:

| Field | Type | Meaning |
|---|---|---|
| `phase` | enum `passed, current, next, upcoming, cancelled` | For timeline highlighting |
| `eta_arrival`, `eta_departure` | **date-time** (RFC 3339, with offset) | Best estimate (3.3). Full timestamps, not `HH:MM`, so that overnight runs and countdowns work. |
| `eta_source` | enum `actual, forecast, propagated, planned` | How the ETA was obtained, so the UI can show "confirmed" vs "estimated" |

Existing `HH:MM` fields stay as they are for backward compatibility. Their spec descriptions will
say they are **Europe/Warsaw local time**.

Data-service `OperationDetail` gets **two** optional fields: `route_id` (int64) and `updated_at`
(date-time). Also `national_number` and `commercial_category_symbol` from the same `routes` join,
so the gateway doesn't need a second call. Nothing else changes there. The ETA/progress logic
lives in the gateway (BFF view logic, like `currentAndNextStations` today) via `shared/trainutil`.

### 3.3 ETA and progress algorithm (`shared/trainutil`, pure, unit-tested)
Input: stops sorted by `actual_sequence_number`, plus `now`, with timestamps as correct UTC
instants (after step 1).

1. **ETA per stop** (arrival and departure separately):
   - `is_confirmed` → `actual_*`, source `actual`.
   - Not confirmed, `actual_*` present → `actual_*`, source `forecast`. This is PLK's forecast,
     validated above.
   - Otherwise, if `planned_*` is present → `planned_* + lastKnownDelay`, source `propagated`.
     `lastKnownDelay` is the delay of the most recent confirmed stop, or 0.
   - Otherwise nil.
   - **Staleness clamp:** a non-confirmed ETA earlier than `now` is lifted to `now`. It is still
     marked `forecast`/`propagated`, with no fake precision.
   - Cancelled stops: no ETA, phase `cancelled`.
   - *Deliberately left out of v1:* delay-recovery heuristics, such as shrinking the delay by
     scheduled dwell slack. PLK's forecast already includes whatever PLK knows. Revisit once
     intraday refresh exists and we can measure error (Q3).
2. **Progress**:
   - `lastConfirmed` = highest-index confirmed, non-cancelled stop.
   - `lastByTime` = highest index whose ETA departure (or arrival for the last stop) ≤ `now`.
   - `last = max(lastConfirmed, lastByTime)`. `basis = confirmed` if `last == lastConfirmed`,
     else `estimated`. This is the dead-reckoning that makes a stale snapshot still move forward.
   - `at_station` if `eta_arrival(last) ≤ now < eta_departure(last)`. `between_stations` if a
     next stop exists. `finished` if `last` is the final stop, or `train_status == C`.
     `cancelled` if `X`. `not_started` if `last` is none.
   - `next` = first non-cancelled stop after `last`.
   - Phases: `< last` → `passed`, `last` → `current`, `next` → `next`, the rest → `upcoming`.
3. **Same function, two consumers.** `feature/train-positions` uses the `(last, next, now)`
   output to interpolate a marker between two stations that have coordinates. Positions on the
   map and the details drawer can then never disagree.

### 3.4 Time zones (shared with `fix/data-freshness`)
- **Storage:** true UTC instants. The plugin interprets offset-less PLK timestamps as
  `Europe/Warsaw` (step 1). A one-off data migration corrects existing rows.
- **Server "now" and service date:** `time.Now()` (an instant). The operating "today" is
  `now.In(Europe/Warsaw)` as a date, not UTC. Put `trainutil.Warsaw()` and
  `trainutil.ServiceDate(now)` in `shared/trainutil`, embedding `time/tzdata` because the gateway
  image is distroless.
- **Display:** `FormatClock` converts to `Europe/Warsaw` before formatting. This **must land in
  the same release as the data fix**: today the display is only correct because both sides are
  wrong the same way. The frontend formats the new date-time fields with
  `Intl.DateTimeFormat('pl-PL', { timeZone: 'Europe/Warsaw' })`.

### 3.5 Freshness
- Daily ingestion makes "current position" meaningless. Recommendation: an **intraday operations
  refresh every 10 min**. The last run took ~2 min to fetch (23 pages) and ~2 min to process, so
  10 min leaves headroom. Disruptions stay daily. Schedules stay weekly, but must actually run (the
  local env is 2 months stale).
- The UI always shows `data_as_of` ("Dane z 14:10"). If `now - data_as_of > 30 min`, it shows a
  warning chip and labels ETAs "szacowane".
- Rejected: calling PLK `/api/v1/operations/train/{scheduleId}/{orderId}/{operatingDate}`
  on demand from the gateway. It breaks the architecture (the gateway must not talk to PLK), and
  it spends API quota per click.

### 3.6 Caching and refresh cadence
- Gateway: no server cache in v1. The response depends on `now`, and a single-run query is cheap
  (two indexed queries). The global `Cache-Control: no-store` stays. A short in-memory TTL can be
  added later if load shows up (Q6).
- Frontend: `refetchInterval: 60_000` while the drawer or page is open (already used). A local
  30 s tick re-renders relative countdowns ("za 7 min") from the `eta_*` timestamps without
  refetching. Stop polling when `progress.state ∈ {finished, cancelled}`.

### 3.7 Frontend UX
**Entry points:**
- A train marker click on the map (from `feature/train-positions`) opens a drawer.
- A live-train row in the map overlay opens the drawer too (it currently links to the full page).
- Station-board rows (`feature/station-board`) link to `/pociagi/{operation_id}`.

**Drawer, not navigation.** State lives in the URL (`/mapa?train=<operationId>`), so it is
shareable, the Back button closes it, and the map keeps its viewport. Desktop: right side panel,
~400 px. Mobile: bottom sheet (the existing overlay pattern in `MapHomeClient.tsx:264`). Esc
closes it and focus returns to the marker.

Drawer content:
1. Header: train name, with a fallback of `train_number` and then "Pociąg #{operation_id}";
   carrier; status badge; current delay badge; `data_as_of` chip.
2. **"Teraz"** card: "Między *Łomża* a *Ostrołęka*" / "Na stacji *X*" / "Nie wyruszył", with
   "(szacunkowo)" when `basis = estimated`.
3. **Next stations:** the next 3 stops with ETA (`HH:MM`), a relative countdown, delay delta vs
   planned, and a "potwierdzone"/"prognoza" marker.
4. Collapsible full timeline, with the shared `TrainTimeline` component highlighting
   passed/current/next phases.
5. Links: "Pełny widok" → `/pociagi/{id}`, "Rozkład" → `/rozklad/{route_id}` (if present).

**Map highlight** while the drawer is open: a polyline through the run's stops that have
coordinates. Coordinates come from the already cached `['mapStations']` query, joined on
`station_external_id`, so the response carries no new coordinate fields. Passed segment solid,
remaining segment dashed. Stops without coordinates are skipped (honest, if coarse, given 93/3,319
coverage). Fit bounds to the polyline on open.

**Full page** `/pociagi/[id]` reuses `TrainTimeline` and the "Teraz" card, and adds a small
non-interactive map preview (optional, Q7).

### 3.8 Disruptions affecting the train (deferred)
The schema supports it (`disruption_affected_routes` has `schedule_id/order_id/operating_date`),
but ingestion is broken on real data (section 2). Later work: a data-service filter
`GET /api/v1/disruptions?scheduleId=&orderId=&operatingDate=`, plus a gateway `disruptions[]`
array on `TrainDetailView`. This is step 8, blocked on fixing disruption ingestion.

## 4. Shared pieces and cross-branch dependencies

| Piece | Owner (proposed) | Consumers | Notes |
|---|---|---|---|
| `operation_id` as the train key | here (documented) | train-positions markers, station-board rows | No code; contract only |
| `shared/trainutil`: `Warsaw()`, `ServiceDate()`, `FormatClock` in Warsaw time | **fix/data-freshness** (or whoever lands first) | all gateway views | Avoid three copies. Coordinate the merge order. |
| `shared/trainutil`: `BuildTimeline(stops, now)` (ETA + progress) | **here** (step 3) | train-positions (marker interpolation), station-board (per-station ETA) | Pure function over `dsmodel.OperationStation` |
| Plugin timestamp fix + data migration | fix/data-freshness **or** here (step 1) | everything time-based | Must ship together with the `FormatClock` change |
| Intraday operations refresh | fix/data-freshness **or** here (step 2) | train-positions, station-board, this view | Without it, all three "live" features are cosmetic |
| `TrainTimeline` component, `useTrainDetail` hook | here (step 5) | station-board could reuse the row styling | |
| Map drawer + `?train=` URL param + `onTrainSelect(operationId)` | here (step 6) | train-positions calls `onTrainSelect` from marker clicks | Agree on the prop name early |
| Frontend `src/lib/` untracked (issue #11) | — | all FE steps | Until #11 is fixed, FE steps must `git add -f services/frontend/src/lib/...` |

Suggested merge order: data-freshness (time utilities, timestamp fix, refresh) → steps 3–4 here →
train-positions (markers) → steps 5–6 here → station-board links.

## 5. Implementation plan

Each step is one small PR-sized unit. Steps 1–2 may be absorbed by `fix/data-freshness`; check
that branch before starting them.

---

### Step 1: Store operation timestamps as correct UTC instants (data-engineering)
**Depends on:** nothing. **Blocks:** step 4's `FormatClock` change (ship together or step 4 first behind the same PR train).

**Implementation prompt** (`data-engineering`):
> In the Pociag do Predykcji repo, read `CLAUDE.md`, `airflow/CLAUDE.md` and
> `db/migrations/CLAUDE.md`. Evidence suggests PLK operation timestamps (`plannedArrival`,
> `plannedDeparture`, `actualArrival`, `actualDeparture`) arrive **without a UTC offset** and are
> Polish local time. `airflow/plugins/pociag_processing/repository.py:39-42` `_parse_timestamp` uses
> `datetime.fromisoformat`, so naive values are stored as UTC. Locally, 9,666 of 18,422 confirmed
> stops have actual times up to 2 h *after* their ingestion time.
> 1. First **verify** the raw format: read one raw operations Parquet envelope from MinIO
>    (`LakeReader.read_raw_operations`) and print a few timestamp strings. Report whether they
>    carry `Z`/offset. If they already carry an offset, stop and report. The bug is then elsewhere.
> 2. If naive: change `_parse_timestamp` so naive datetimes get `ZoneInfo("Europe/Warsaw")`
>    attached. Aware values stay unchanged. This also applies to any other caller of
>    `_parse_timestamp`; check them.
> 3. Add migration `db/migrations/010_operation_timestamps_warsaw.up.sql` / `.down.sql`. It
>    converts existing `operation_stations.planned_arrival, planned_departure, actual_arrival,
>    actual_departure` with `(col AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw'`, and the down
>    migration applies the inverse. Wrap it in a transaction. Say in a header comment that this is
>    a one-off data correction.
> 4. Tests in `airflow/tests/test_repository.py`: naive → Warsaw-aware (summer and winter
>    offsets), aware input unchanged, None → None.
> Acceptance: `uv run ruff check .`, `uv run mypy plugins/pociag_processing` and
> `uv run pytest tests/ -v` are green. After `make db-migrate-up` and a re-run of the operations
> DAG, this returns ~0:
> `SELECT count(*) FROM operation_stations os JOIN train_operations t ON t.id=os.train_operation_id WHERE os.is_confirmed AND COALESCE(os.actual_departure, os.actual_arrival) > t.updated_at + interval '5 minutes';`
> Touch only `repository.py`, the two migration files and the test file.

### Step 2: Intraday operations refresh (data-engineering; devops-infra for scheduling and ops)
**Depends on:** nothing (step 1 recommended). **Blocks:** meaningful "current" data.

**Implementation prompt** (`data-engineering`):
> Read `CLAUDE.md` and `airflow/CLAUDE.md`. Operations are ingested only once a day
> (`airflow/dags/ingest_operations_daily.py:57`, `schedule="0 2 * * *"`), so "current position"
> views are hours stale. Add a new DAG `airflow/dags/refresh_operations_intraday.py` that runs
> every 10 minutes (`schedule="*/10 * * * *"`, `catchup=False`, `max_active_runs=1`,
> `dagrun_timeout=timedelta(minutes=9)`). It performs only the operations fetch
> (`POST /api/v1/fetch/operations` via the `pociag_collector` connection) and
> `process_operations(capture_date, ingestion_run_id)`, reusing the existing task pattern. Do not
> fetch disruptions. Compute `capture_date` as today in `Europe/Warsaw`, not `date.today()`. Leave
> the daily DAG in place. Check that `repository.is_pipeline_running` and the collector's
> `IsPipelineRunning` guard behave correctly with many runs per date: a stuck `running` row must
> not block all later runs. If it can, report it; don't redesign it.
> Tests: a unit test for the capture-date helper (UTC 23:30 on the last Saturday of March →
> Warsaw date is the next day). Acceptance: ruff/mypy/pytest green. The DAG parses
> (`list_dags`/`get_import_errors`). A manual trigger succeeds in under 5 min against the local
> stack.

### Step 3: `shared/trainutil` timeline and ETA engine (go-backend)
**Depends on:** nothing (pure code). **Blocks:** step 4, and `feature/train-positions` marker logic.

**Implementation prompt** (`go-backend`):
> Read `CLAUDE.md` and `services/go/CLAUDE.md`. In `services/go/shared/trainutil/`, add a new file
> `timeline.go`; do not edit unrelated functions in `util.go`. Contents:
> - `func Warsaw() *time.Location` (`time.LoadLocation("Europe/Warsaw")` once via `sync.Once`;
>   `import _ "time/tzdata"` so the distroless image works) and
>   `func ServiceDate(now time.Time) string` (Warsaw calendar date, `2006-01-02`). **Skip these two
>   if `fix/data-freshness` already added equivalents; reuse theirs.**
> - Types `StopPhase` (`passed,current,next,upcoming,cancelled`), `ETASource`
>   (`actual,forecast,propagated,planned`), `ProgressState`
>   (`not_started,at_station,between_stations,finished,cancelled,unknown`),
>   `StopEstimate{Index int; Phase StopPhase; ETAArrival, ETADeparture *time.Time; Source ETASource}`,
>   `Timeline{Stops []StopEstimate; LastIndex, NextIndex int /* -1 if none */; State ProgressState; Basis string /* confirmed|estimated */; CurrentDelayMinutes *int}`.
> - `func BuildTimeline(stops []dsmodel.OperationStation, trainStatus string, now time.Time) Timeline`,
>   implementing exactly this algorithm. Stops are sorted by `ActualSequenceNumber` on a copy.
>   ETA per stop: confirmed → actual (`actual`). Unconfirmed with actual → actual (`forecast`).
>   Else planned + delay of last confirmed stop (`propagated`), with 0 when unknown (`planned`).
>   A non-confirmed ETA earlier than `now` is clamped to `now`. Cancelled stops get no ETA and
>   phase `cancelled`. `last = max(last confirmed non-cancelled, last index whose ETA departure
>   (arrival for final stop) <= now)`. `Basis` is `confirmed` iff `last` equals the last confirmed
>   index. State: status `X` → cancelled; `C` or last is final stop → finished; last == -1 →
>   not_started; `ETAArrival(last) <= now < ETADeparture(last)` → at_station; otherwise
>   between_stations. `next` = first non-cancelled after `last`. `CurrentDelayMinutes` uses the
>   same rule as `currentAndNextStations` in `gateway/internal/service/service.go:811-851`.
> - If `shared/dsmodel` is not importable from `trainutil` without a cycle, define a minimal
>   input struct instead and document it.
> Tests `timeline_test.go` (testify, table-driven, `t.Parallel()`): not started, mid-run between
> stations, dwelling at a station, stale snapshot (confirmed stop 2, but `now` past stop 4's ETA →
> last=4, basis=estimated), cancelled middle stop skipped for `next`, final stop reached, status X,
> missing actuals → propagated, overnight run crossing midnight, Warsaw DST boundary for
> `ServiceDate`. Acceptance: `cd services/go/shared && go test -race ./...` green, and
> `make gateway-test` still green. No other files touched.

### Step 4: Contract and implementation for ETA/progress on train detail (go-backend)
**Depends on:** step 3. Step 1 before or in the same release (because of the `FormatClock` change).

**Implementation prompt** (`go-backend`):
> Read `CLAUDE.md`, `services/go/CLAUDE.md`, `services/go/data-service/CLAUDE.md` and
> `services/go/gateway/CLAUDE.md`. Contract-first: edit specs before code.
> 1. `specs/openapi/data-service.yml` `OperationDetail` (~line 905): add optional `route_id`
>    (int64), `national_number` (string), `commercial_category_symbol` (string) and `updated_at`
>    (date-time, "when this run was last ingested").
> 2. `specs/openapi/gateway.yml`: on `TrainDetailView` (~line 687), add optional `route_id`,
>    `train_number`, `commercial_category`, `origin`, `destination`, `current_delay_minutes`,
>    `data_as_of` (date-time), `generated_at` (date-time), and
>    `progress {state enum[not_started,at_station,between_stations,finished,cancelled,unknown] (required), basis enum[confirmed,estimated] (required), last_stop_sequence int, next_stop_sequence int}`.
>    On `TrainStopView` (~line 655), add optional `phase` enum
>    `[passed,current,next,upcoming,cancelled]`, `eta_arrival`/`eta_departure` (date-time) and
>    `eta_source` enum `[actual,forecast,propagated,planned]`. Update the existing `HH:MM` field
>    descriptions to say "Europe/Warsaw local time". Keep `additionalProperties: false`. Bump
>    `info.version` minor.
> 3. data-service: extend `GetOperationById` in `internal/repository/operations.go:160-211` to
>    select `MAX(r.id)`, `MAX(r.national_number)`, `MAX(r.commercial_category_symbol)` and
>    `to2.updated_at`. Add the new fields to the data-service model and `shared/dsmodel.OperationDetail`
>    (pointer/omitempty). Parameterized SQL only.
> 4. gateway: in `GetTrainDetail` (`internal/service/service.go:374-430`) call
>    `trainutil.BuildTimeline(detail.Stations, detail.TrainStatus, s.now())` and map it onto the new
>    fields. Inject a clock (`now func() time.Time` on `Service`, default `time.Now`) so it can be
>    tested; don't change `New`'s signature beyond what's needed. Change `trainutil.FormatClock` to
>    format in `trainutil.Warsaw()` (**only if step 1 is merged/included, and coordinate with
>    fix/data-freshness if it already did this**). Extend the gateway model structs and swag
>    comments. Fall back so `train_name` is never empty: route name → `national_number` → "".
>    The frontend handles "".
> Tests: `gateway/internal/service/service_test.go`. Using the existing fake client pattern, cover
> the progress/phase mapping, the ETA source mapping, a missing route (nil `route_id`), and
> `data_as_of` passthrough. Acceptance: `make data-service-test data-service-lint gateway-test gateway-lint`
> green. `curl localhost:8084/api/v1/trains/<id>` against the local stack shows the new fields.
> Existing fields are unchanged except the documented time zone.

### Step 5: Frontend timeline upgrade on `/pociagi/[id]` (frontend-design)
**Depends on:** step 4 (the gateway contract). Can start against the spec with mocked data.

**Implementation prompt** (`frontend-design`):
> Read `CLAUDE.md` and `services/frontend/CLAUDE.md`. Note issue #11: `services/frontend/src/lib/`
> is ignored by a root `.gitignore` rule. If it is missing in your worktree, copy `api.ts`/`ui.tsx`
> from the main checkout, and stage your edits with `git add -f`.
> 1. `src/lib/api.ts`: extend the `TrainStopView`/`TrainDetailView` interfaces with the new
>    optional fields from `specs/openapi/gateway.yml` (`phase`, `eta_arrival`, `eta_departure`,
>    `eta_source`, `route_id`, `train_number`, `origin`, `destination`, `current_delay_minutes`,
>    `progress`, `data_as_of`, `generated_at`). Add `useTrainDetail(operationId, { enabled })`, a
>    react-query hook with `refetchInterval` 60 s that stops when
>    `progress.state ∈ {finished,cancelled}`. Add helpers `formatWarsawClock(iso)` and
>    `formatRelativeEta(iso, now)` (Intl, `timeZone: 'Europe/Warsaw'`, pl-PL).
> 2. Extract `StopTimeline` from `src/app/pociagi/[id]/page.tsx:17-77` into
>    `src/components/TrainTimeline.tsx`. Highlight by `phase`: passed = muted with check; current =
>    pulsing accent ring; next = accent border; cancelled = strike-through (as now). Show planned
>    time, ETA (`HH:MM`) when it differs, a "prognoza"/"potwierdzone" label from `eta_source`, and
>    the delay badge. Prop `compact?: boolean` collapses passed stops to "N stacji za nami" with an
>    expand toggle.
> 3. Add `src/components/TrainNowCard.tsx`, which renders `progress` ("Między A a B", "Na stacji
>    X", "Nie wyruszył", "Zakończył bieg", "(szacunkowo)" if basis=estimated), the next 3 stops with
>    ETA and a relative countdown (local 30 s tick), and a freshness chip from `data_as_of` that
>    turns amber when it's more than 30 min old.
> 4. Update `src/app/pociagi/[id]/page.tsx` to use the hook and both components. Show
>    "Rozkład" → `/rozklad/{route_id}` when present, and use the fallback title
>    `train_name || train_number || 'Pociąg #' + operation_id`.
> Accessibility: timeline is an `<ol>`; the current stop has `aria-current="step"`; colors are
> never the only signal. Acceptance: `npm run build` is green. Manual check against the local
> stack: passed, current and next are visibly distinct; an old `data_as_of` shows the warning.
> Touch only the files listed.

### Step 6: Map drawer and route highlight (frontend-design)
**Depends on:** step 5. Integrates with `feature/train-positions` through one prop/callback.

**Implementation prompt** (`frontend-design`):
> Read `CLAUDE.md` and `services/frontend/CLAUDE.md` (and the issue #11 note in step 5). Add a
> train details drawer to the map:
> 1. `src/components/TrainDetailsDrawer.tsx`, a client component taking
>    `{ operationId: number; onClose(): void }`. It uses `useTrainDetail`, `TrainNowCard` and
>    `TrainTimeline compact`, plus links "Pełny widok" → `/pociagi/{id}` and "Rozkład" →
>    `/rozklad/{route_id}`. Desktop: right-side panel (~400 px, `md:` breakpoint) over the map.
>    Mobile: bottom sheet reusing the visual language of the overlay in
>    `MapHomeClient.tsx:264`. Esc and the close button close it; focus moves into the drawer on
>    open and back to the trigger on close. Loading, error (retry) and 404 ("Nie znaleziono
>    pociągu") states.
> 2. State in the URL: `/mapa?train=<operationId>` via `useSearchParams`/`router.replace`
>    (`push` on open so Back closes it). `MapHomeClient` reads the param and renders the drawer.
>    While it's open, hide or collapse the left operations panel on mobile.
> 3. Export `openTrain(operationId)` from a small `src/lib/useSelectedTrain.ts` hook, so
>    `feature/train-positions` markers can call it from their click handler. Change `TrainRow` in
>    `MapHomeClient.tsx:222-236` to open the drawer instead of navigating. Keep a real
>    `href="/mapa?train=…"` for middle-click.
> 4. Route highlight in `TrafficMapClient.tsx`: accept an optional `highlight?: { stops:
>    TrainStopView[] }` prop. Join `station_external_id` with the cached `['mapStations']` data
>    and draw a `Polyline` in a new pane above the stations pane (zIndex 425). Passed segment
>    solid, remaining dashed, current/next stations with a larger ring. Fit bounds on open
>    (padding for the drawer). Skip stops without coordinates. If fewer than 2 have coordinates,
>    draw nothing and show "Brak współrzędnych trasy" in the drawer.
> 5. Update the disclaimer text at `MapHomeClient.tsx:296` only if train-positions has not
>    already changed it.
> Acceptance: `npm run build` is green. Manual check: clicking a live-train row opens the drawer,
> the URL updates, Back closes it, a reload with `?train=` reopens it, the route is highlighted
> and the map fits it, and a 375 px-wide viewport is usable. Only the listed files are touched.

### Step 7: Station-board linking (frontend-design; contract only here)
Owned by `feature/station-board`. The requirement for that branch: each board row exposes
`operation_id` and links to `/pociagi/{operation_id}`, or opens `/mapa?train={operation_id}` if
the board is on the map. No work in this branch.

### Step 8 (deferred): Disruptions affecting this run (data-engineering → go-backend → frontend-design)
**Blocked on** fixing disruption ingestion (section 2). Outline: the data-service spec plus a
`scheduleId, orderId, operatingDate` filter on `GET /api/v1/disruptions`, backed by
`disruption_affected_routes` (add an index on `(schedule_id, order_id, operating_date)` in
migration 011). Gateway: optional `disruptions[] {id, type_name, message, station_name}` on
`TrainDetailView`. Frontend: an alert strip in the drawer. Write the full prompts once ingestion
is fixed.

## 6. Alternatives considered

| Option | Why not (for now) |
|---|---|
| New endpoint `/trains/{id}/progress` | Two requests per drawer and duplicated stop data. The additive extension is backward compatible. |
| ETA/progress in data-service SQL | Depends on `now`, and it's view logic. It's easier to unit-test in Go, and it has to be shared with train-positions marker placement in the gateway anyway. |
| Own delay-prediction model | PLK already forecasts downstream stops. Measure first (Q3). |
| On-demand PLK single-train fetch | Breaks the "gateway never talks to PLK" architecture and spends API quota. |
| Navigate to `/pociagi/[id]` on marker click | Loses map context. The user asked to "open it by clicking on the train (on the map)". The drawer keeps the full page one click away. |
| Carry lat/lon on `TrainStopView` | The frontend already caches `/map/stations`, so a join avoids widening the contract. |

## 7. Open questions / decisions for you

1. **Q1: Who owns the timestamp fix and the intraday refresh (steps 1–2)?** This branch, or
   `fix/data-freshness`? They block all three "live" features.
2. **Q2: Refresh cadence.** Is 10 min acceptable, and is the PLK API quota enough for
   ~144 × 23 page requests per day? Or 5 min?
3. **Q3: ETA heuristics.** Is PLK's forecast (plus clamping to `now`) enough for v1, or do you
   want a delay-recovery heuristic now? I recommend measuring forecast error once intraday data
   exists.
4. **Q4: Stale runs.** Should the view refuse to show "live" state for runs whose
   `operating_date` is older than yesterday in Warsaw time, even if PLK still reports `P`? Or is
   that `fix/data-freshness`'s filter?
5. **Q5: Identifier durability.** Is `operation_id` in shareable URLs fine, or do links need to
   survive a DB rebuild, which would need the natural-key route
   `/pociagi/{scheduleId}-{orderId}-{date}`?
6. **Q6: Gateway caching.** Start without a cache (recommended), or add a 15 s in-memory TTL per
   operation now?
7. **Q7: Map preview** on the standalone `/pociagi/[id]` page: wanted in v1?
8. **Q8: Local data.** Schedules were last ingested 2026-07-19, so train names are empty for
   today's runs. Should the weekly schedules DAG be re-run / checked before the demo?
