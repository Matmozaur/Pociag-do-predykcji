# Proposal: data freshness of "live" trains

Status: **proposal, nothing implemented yet** · Branch: `fix/data-freshness` · Date: 2026-09-27

## 1. Problem

User report: *"There seems to be a problem with data freshness, I see routes that ended hours ago
shown as current."*

"Current" trains show up in two places in the frontend, and both take their data from the gateway:

- `/mapa` side panel: "Pociągi w ruchu" list plus the "W ruchu" KPI
  (`services/frontend/src/components/MapHomeClient.tsx:79-80,106`)
- `/pociagi` "Pociągi na żywo" (`services/frontend/src/app/pociagi/page.tsx:79-84`)

Both call `GET /api/v1/trains/live` or `GET /api/v1/dashboard/overview` on the gateway.

## 2. How "current" is determined today (end-to-end)

| Layer | What it does | Evidence |
|---|---|---|
| PLK API | `GET /api/v1/operations` returns a **real-time snapshot** ("wykonanie pociągów w czasie rzeczywistym"). `trainStatus` is the status at the moment of the call. | `specs/openapi/plk-open-data.json` (`/api/v1/operations`) |
| collector | `POST /api/v1/fetch/operations` pulls every page of that snapshot and stores it under `raw/operations/YYYY/MM/DD/run_<id>_page_<n>` (capture date is **UTC**). | `services/go/collector/internal/service/service.go:186-226`, `internal/plk/client.go:77-83` |
| Airflow | `ingest_operations_daily` runs **once a day** at `0 2 * * *` (02:00 UTC). It is the only thing that refreshes `train_operations`. The spec describes this DAG as *"Pull yesterday's completed operations for historical analysis"*. | `airflow/dags/ingest_operations_daily.py:55-62`, `specs/airflow-dags.md` (DAG 3) |
| plugin | `upsert_operations` overwrites `train_status` and the stop times with whatever the snapshot said. Naive PLK timestamps are stored via `datetime.fromisoformat` without a timezone. | `airflow/plugins/pociag_processing/repository.py:39-42,564-650` |
| data-service | `QueryOperations` filters only by `operating_date = $date` and `train_status = $status`. It has no notion of time. | `services/go/data-service/internal/repository/operations.go:23-40` |
| gateway | "live" = `date = time.Now().UTC()` + `status = "P"`. Overview "in progress" = `ByStatus["P"]` for the UTC date. `generated_at` = `time.Now()`, not the time of the data. `data_freshness` is always empty. | `services/go/gateway/internal/service/service.go:322-370` (`:326`, `:330`, `:369`), `:540-578` (`:544`, `:568`, `:577`) |
| frontend | Polls every 60 s and shows "Odświeżono: HH:mm:ss" using the **client fetch time** (`dataUpdatedAt`), which suggests the data is fresh. | `services/frontend/src/app/pociagi/page.tsx:59-65,92-96`, `MapHomeClient.tsx:80` |
| caching | Not a cause. The gateway sends `Cache-Control: no-store` by default. The Next.js `revalidate: 30` and the react-query `staleTime` are 30-60 s. | `services/go/gateway/internal/config/config.go:43`, `services/go/gateway/internal/middleware/cache.go` |

## 3. Root causes

### RC1 (primary, confirmed): the `P` status is a stale snapshot, but it is served as "live"

`train_status` is only as fresh as the last `ingest_operations_daily` run. Once a train is captured
as `P` (in progress), it stays `P` in PostgreSQL until the next run, which is up to 24 h later. The
gateway treats `status = 'P'` as "running now" with no time-based check.

Query evidence (local stack, `now()` = 2026-09-27 15:20 UTC / 17:20 Warsaw):

- The last operations upsert was at `06:11–06:13 UTC`: `max(train_operations.updated_at)`, and the
  `ingestion_runs` row for `operations` with `completed_at 06:13:20Z`. That is about **9 h stale**.
- `operating_date = 2026-09-27` has **506 rows with `train_status = 'P'`**. Of these:
  - 490 have their last stop time (`COALESCE(actual_arrival, planned_arrival, planned_departure)`)
    already in the past.
  - **439 finished more than 3 h ago.** These are exactly the "routes that ended hours ago shown as
    current".
- The same pattern holds for earlier dates: `P` rows for 09-24, 09-25 and 09-26 (211/206/…) were
  never moved to `C`.

### RC2 (confirmed): PLK itself keeps stale `P` records, so status alone is not trustworthy

The raw snapshot from 2026-09-27 (read-only via `LakeReader` in the Airflow container) contained
`('2026-09-24','P'): 446`, `('2026-09-25','P'): 422`, `('2026-09-26','P'): 412`. Those are trains from
previous days that PLK still reports as in progress. Refreshing more often is therefore not enough
on its own. "Live" also needs a **time-based guard**: the expected arrival at the last stop must not
be in the past beyond a grace period.

### RC3 (confirmed): PLK local wall-clock times are stored as UTC, off by 1–2 h

PLK operations timestamps are **naive Warsaw wall-clock** values. A raw sample:
`'plannedArrival': '2026-09-28T10:30:00', 'plannedArrivalTime': '10:30:00'`, with no offset.
Two things contribute:

- The OpenAPI schema claims "UTC", but the raw data is naive local time.
- `_parse_timestamp` (`repository.py:39-42`) returns a naive `datetime`, and Postgres interprets it
  in session `TimeZone = UTC`.

The result is that `operation_stations.planned_*/actual_*` (TIMESTAMPTZ,
`db/migrations/003_operations.up.sql:32-37`) hold instants that are **2 h late in summer and 1 h
late in winter**.

Evidence: across 18,807 first stops on 2026-09-24..26, `(planned_departure AT TIME ZONE
'UTC')::date <> operating_date` in **0** cases. `AT TIME ZONE 'Europe/Warsaw'` gives **876**
mismatches. So the stored UTC wall clock *is* the Warsaw wall clock.

Display looks correct only by accident. `trainutil.FormatClock` formats in the parsed offset, which
is UTC (`services/go/shared/trainutil/util.go:51-57`). Once any "now"-based logic is added (RC1 fix),
this becomes a real bug. It also means **RC3 and the display fix must ship together**. Otherwise
the UI will show times 2 h early.

### RC4 (confirmed): "today" is the UTC date, not the Warsaw date

The gateway uses `time.Now().UTC().Format("2006-01-02")` for live trains, the overview and active
disruptions (`service.go:326,437,544`). Between 00:00 and 02:00 Warsaw (CEST), this selects
**yesterday's** operations, including all their stale `P` rows. The same issue exists in the
collector capture date (`collector/.../service.go:191`) and the DAG (`date.today()` on a UTC
worker, `ingest_operations_daily.py:51-52`). Those two agree with each other, so they are harmless
for correctness today.

### RC5 (confirmed, UX): freshness is not communicated

- `DashboardOverview.data_freshness` is specified (`specs/openapi/gateway.yml:833-843`) but always
  returned empty (`service.go:577`).
- `LiveTrainsResponse.generated_at` is described as "Timestamp of data (for freshness
  indication)" (`gateway.yml:649-652`) but set to `time.Now()`.
- The frontend shows client fetch time as "Odświeżono". The user has no way to tell that the data
  is 9 h old.

## 4. Proposed fix: sequenced steps

Contract-first ordering: each step that changes a contract edits the spec in the same PR, before the
code. Steps 1 and 2 are the minimum to make the symptom go away. Steps 3 and 4 must ship together.

| # | Step | Agent | Contract change | Depends on |
|---|---|---|---|---|
| 1 | Frequent operations ingestion (new `ingest_operations_live` DAG) | data-engineering | `specs/airflow-dags.md` | – |
| 2 | Time-guarded "live" filter in data-service + Warsaw "today" in gateway | go-backend | `specs/openapi/data-service.yml` (new query param) | – (works better with 1) |
| 3 | Store PLK operation timestamps as correct instants (+ data migration) | data-engineering | `specs/schemas/train-operation.json` description only | ship with 4 |
| 4 | Render clock times in Europe/Warsaw | go-backend | – | ship with 3 |
| 5 | Expose real data freshness (`operations_last_updated`, `generated_at`) | go-backend | `specs/openapi/data-service.yml`, `gateway.yml` description | – |
| 6 | Show data age + staleness warning in UI | frontend-design | – | 5 |

Note on Step 2 before 3/4: Step 2's guard compares stop timestamps with `now()`. While RC3 is
unfixed, stored instants are 1–2 h *late*, so the guard is **conservative**: a finished train
lingers 1–2 h longer than it should. It never hides a running train. It is safe to ship first.
The grace window can be tightened after 3/4.

---

### Step 1: Frequent operations ingestion (data-engineering)

Why: RC1. Daily snapshots cannot represent "live". Each fetch is about 46 PLK pages / 1000 rows
(about 2 min end-to-end today). At 10-minute cadence that is about 280 req/h, within PLK tier
limits (100–2000 req/h). **Confirm the API key's tier; see open questions.**

**Implementation prompt (subagent: `data-engineering`):**

> Repo "Pociag do Predykcji". Read `CLAUDE.md` and `airflow/CLAUDE.md`. Work on a `fix/` branch.
>
> Goal: refresh `train_operations` frequently so the "live trains" view reflects the current PLK
> snapshot. Today only `airflow/dags/ingest_operations_daily.py` (cron `0 2 * * *`) fetches
> operations.
>
> 1. **Spec first:** in `specs/airflow-dags.md` add a DAG section `ingest_operations_live`:
>    schedule `*/10 * * * *`, `catchup=False`, `max_active_runs=1`, retries 1 / 1 min,
>    `dagrun_timeout` 9 min, tags `["pociag","ingestion"]`. Tasks: `fetch_operations`
>    (`POST /api/v1/fetch/operations` `{"force": false}` via the `pociag_collector` connection) →
>    `process_operations` (plugin `process_operations(capture_date, ingestion_run_id)`). Note that
>    the daily DAG remains for disruptions and the end-of-day snapshot.
> 2. Create `airflow/dags/ingest_operations_live.py`. Mirror the fetch/process tasks of
>    `ingest_operations_daily.py` exactly (same TypedDicts pattern, `_fetch_run_id`,
>    `capture_date_today`). Reuse by copying the small helpers. Do **not** refactor the daily DAG
>    or create a shared module. Always pass the `ingestion_run_id` so only the latest run's
>    objects are processed.
> 3. If a run is skipped because the collector returns 409 / "pipeline running", the task should
>    log and succeed (not fail and retry-storm). Check how the collector surfaces
>    `ErrPipelineRunning` in `services/go/collector/internal/handler/handler.go`.
> 4. Do not change the collector, the plugin, or migrations.
>
> Acceptance: `cd airflow && uv run ruff check . && uv run mypy plugins/pociag_processing && uv run
> pytest tests/ -v` all pass. The DAG parses (check with `list_dags`/`get_import_errors` if the
> stack is running; do not trigger it without the user's OK). Tests: add a unit test that imports
> the DAG module and asserts `schedule`, `max_active_runs == 1`, `catchup is False`, and the task
> ids/order (follow the existing test style, no DagBag needed if none exists; mock `httpx.post`
> and `BaseHook.get_connection` for a fetch-task test including the 409 path).

---

### Step 2: Time-guarded "live" filter + Warsaw "today" (go-backend)

Why: RC1, RC2 and RC4. Even with frequent refresh, PLK leaves stale `P` rows, and overnight trains
cross midnight.

Definition of *live* (see open question Q1):
`train_status = 'P'` **and** `operating_date ∈ {today_Warsaw, today_Warsaw − 1}` **and** the
train's **expected end** `>= now() − grace`. Expected end is
`MAX(COALESCE(actual_arrival, planned_arrival + arrival_delay_minutes·1min, planned_arrival,
planned_departure))` over its stops. The default grace is 30 min.

**Implementation prompt (subagent: `go-backend`):**

> Repo "Pociag do Predykcji". Read `CLAUDE.md`, `services/go/CLAUDE.md`,
> `services/go/data-service/CLAUDE.md`, `services/go/gateway/CLAUDE.md`. Work on a `fix/` branch.
> Keep the diff minimal.
>
> Problem: `/api/v1/trains/live` shows trains that finished hours ago. The gateway
> (`services/go/gateway/internal/service/service.go` `GetLiveTrains`, around line 322) asks
> data-service for `date=<UTC today>&status=P`. `train_status` is a stale PLK snapshot and PLK
> never closes some `P` rows.
>
> 1. **Spec first:** in `specs/openapi/data-service.yml`, `GET /api/v1/operations`, add an
>    optional boolean query param `activeOnly` (default false): "When true, return only operations
>    with status P whose expected end (last stop's actual arrival, else planned arrival + arrival
>    delay, else planned arrival/departure) is not earlier than now minus 30 minutes; when `date`
>    is also given, the previous operating date is included to cover overnight trains." Mirror
>    the param in the gateway spec description of `/api/v1/trains/live` (text only, no new gateway
>    params).
> 2. data-service: thread `ActiveOnly bool` through handler → `service.QueryOperationsParams` →
>    `repository.QueryOperations` (`internal/repository/operations.go`). When set, add conditions
>    using the existing `$n` param-builder style:
>    - `to2.train_status = 'P'` (use a param, not a literal, to match the style)
>    - if `Date` set: `to2.operating_date IN ($d::date, $d::date - 1)` instead of the equality
>    - `EXISTS`-free guard via a correlated subquery:
>      `(SELECT MAX(COALESCE(os4.actual_arrival, os4.planned_arrival + make_interval(mins => COALESCE(os4.arrival_delay_minutes,0)), os4.planned_departure)) FROM operation_stations os4 WHERE os4.train_operation_id = to2.id) >= now() - $n::interval`
>      Pass the grace as a Go constant (`30 * time.Minute` → `'30 minutes'`), not config. Do not
>      add a new env var.
>    Order the live list by the same `to2.id` ordering (do not change ordering for non-live
>    callers).
> 3. gateway: add `ActiveOnly` to `dataservice.QueryOperationsParams` and the client query
>    encoding. In `GetLiveTrains` pass `ActiveOnly: true` and compute `today` as the
>    **Europe/Warsaw** date. Add a small unexported helper `warsawToday(now time.Time) string` in
>    the gateway service package using `time.LoadLocation("Europe/Warsaw")`, and add
>    `import _ "time/tzdata"` in `gateway/cmd/main.go` (the runtime image is distroless, so tzdata
>    must be embedded). Use the same helper for `ListDisruptions(active)` and
>    `GetDashboardOverview`. Do not change overview semantics beyond the date.
>
> Acceptance / tests:
> - `make data-service-test data-service-lint gateway-test gateway-lint` pass.
> - data-service: handler test that `activeOnly=true` is parsed and invalid values give 400.
>   Repository unit test (if a pattern exists; otherwise a service-level test with a fake repo)
>   that the param is propagated.
> - gateway: table test for `warsawToday` at 2026-09-27T22:30Z → `2026-09-28` (CEST) and
>   2026-01-15T23:30Z → `2026-01-16` (CET). Test that `GetLiveTrains` sends `ActiveOnly=true` and
>   the Warsaw date to the client fake.
> - Manual check (read-only SQL) that the equivalent query on the local DB returns far fewer rows
>   than `status='P'` alone (about 16 of 506 in the 2026-09-27 snapshot).

---

### Step 3: Correct instants for PLK operation timestamps (data-engineering) *(ship with Step 4)*

Why: RC3. The stored instants are 1–2 h late. Any `now()` comparison (Step 2) and any
external consumer are skewed.

**Implementation prompt (subagent: `data-engineering`):**

> Repo "Pociag do Predykcji". Read `CLAUDE.md`, `airflow/CLAUDE.md`, `db/migrations/CLAUDE.md`.
> Work on a `fix/` branch. This PR must be merged together with the go-backend PR that renders
> clock times in Europe/Warsaw (Step 4), otherwise the UI will show times 1–2 h early.
>
> Problem: PLK `/api/v1/operations` stop timestamps (`plannedArrival`, `plannedDeparture`,
> `actualArrival`, `actualDeparture`) are **naive Europe/Warsaw wall-clock** strings
> (e.g. `"2026-09-28T10:30:00"`). `airflow/plugins/pociag_processing/repository.py`
> `_parse_timestamp` (line ~39) returns a naive datetime, and Postgres (session TZ UTC) stores it
> as UTC into TIMESTAMPTZ columns of `operation_stations`. The result is 2 h late in summer and
> 1 h late in winter.
>
> 1. **Spec:** in `specs/schemas/train-operation.json` (and the `OperationStation` time fields in
>    `specs/openapi/data-service.yml` if present), clarify in `description` only that the values
>    are UTC instants and PLK naive values are interpreted as Europe/Warsaw. No shape change.
> 2. In `repository.py` add `_parse_plk_local_timestamp(value)`. If the parsed datetime is naive,
>    attach `zoneinfo.ZoneInfo("Europe/Warsaw")`. If it already has an offset, keep it. Use it
>    **only** for the four operation stop fields in `upsert_operations`. Leave `_parse_timestamp`
>    and the carriers `validFrom/validTo` usage untouched (file an issue if you think those are
>    affected too). DST ambiguity: rely on zoneinfo default `fold=0`, and document it in a
>    one-line comment.
> 3. Data migration `db/migrations/010_operation_stations_warsaw_tz.{up,down}.sql` (check the
>    next free number). `up` converts existing rows:
>    `SET col = (col AT TIME ZONE 'UTC') AT TIME ZONE 'Europe/Warsaw'` for the 4 columns, in one
>    `UPDATE` inside `BEGIN/COMMIT`. `down` does the inverse. Note in the migration header that
>    it is a one-shot correction and must run exactly once, together with the plugin release.
>    Ask the user before applying it to any DB.
>
> Acceptance / tests: ruff, mypy strict, pytest pass. Update
> `tests/test_repository.py::test_upsert_operations_executes_correct_queries` expectations. Add
> tests: naive `"2026-07-01T10:30:00"` → `2026-07-01T08:30:00Z`; `"2026-01-15T10:30:00"` →
> `09:30Z`; an offset-aware `"2025-06-01T08:00:00+02:00"` unchanged; `None` → `None`. After
> migration, the read-only check
> `SELECT count(*) FILTER (WHERE (planned_departure AT TIME ZONE 'Europe/Warsaw')::date <> operating_date) … WHERE planned_sequence_number = 1`
> should be about 0.

---

### Step 4: Render clock times in Europe/Warsaw (go-backend) *(ship with Step 3)*

**Implementation prompt (subagent: `go-backend`):**

> Repo "Pociag do Predykcji". Read `CLAUDE.md`, `services/go/CLAUDE.md`,
> `services/go/shared/CLAUDE.md`. Work on a `fix/` branch that merges together with the
> data-engineering PR that stores correct UTC instants (Step 3).
>
> `services/go/shared/trainutil/util.go` `FormatClock` formats `ts.Format("15:04")` in whatever
> location the time carries (UTC after JSON). Stored instants will now be correct UTC, so
> formatting must convert to Europe/Warsaw: `ts.In(warsaw).Format("15:04")`. Load the location
> once (package-level `sync.Once` or `var`, falling back to `time.UTC` on error, logged
> nowhere, since shared has no logger). Ensure every binary that formats clocks embeds tzdata
> (`import _ "time/tzdata"` in `gateway/cmd/main.go`; add it to data-service's main only if it
> calls `FormatClock`). Do not change the function signature.
>
> Acceptance / tests: `go test ./...` in `shared`, `gateway`, `data-service`, and lint clean.
> Add a table test in `shared/trainutil` with `2026-07-01T08:30:00Z` → `"10:30"`,
> `2026-01-15T09:30:00Z` → `"10:30"`, and `nil` → `nil`. Fix any existing gateway tests that
> assumed UTC formatting.

---

### Step 5: Expose real data freshness (go-backend)

Why: RC5. The contract already has `data_freshness.operations_last_updated`. It only needs data.

**Implementation prompt (subagent: `go-backend`):**

> Repo "Pociag do Predykcji". Read `CLAUDE.md` and the Go CLAUDE.md files. Work on a `fix/` branch.
>
> 1. **Spec first:** `specs/openapi/data-service.yml` `OperationStatistics`: add optional
>    `last_updated_at` (string, date-time, nullable), described as "MAX(train_operations.updated_at)
>    for the requested date: when operations for this date were last refreshed from PLK".
>    `specs/openapi/gateway.yml`: reword `LiveTrainsResponse.generated_at` to "Time the response
>    was generated (server clock)". Do not repurpose it. Freshness of the data is carried by
>    `DashboardOverview.data_freshness.operations_last_updated`. No other new fields.
> 2. data-service `GetOperationStatistics` (`internal/repository/operations.go` ~line 299): add
>    `MAX(to2.updated_at)` to the CTE/select and map it to the model/JSON.
> 3. gateway: add the field to `dataservice.OperationStatistics` and set
>    `DataFreshness.OperationsLastUpdated` in `GetDashboardOverview` (`service.go` ~line 577).
>    Leave `SchedulesLastUpdated` nil (out of scope).
>
> Acceptance / tests: lint and tests pass. Update the gateway test at `service_test.go:259` that
> currently asserts both freshness fields are nil, so it expects `operations_last_updated` to be
> populated from the fake client. Add a data-service test for the new field's mapping.

---

### Step 6: Show data age and a staleness warning (frontend-design)

**Implementation prompt (subagent: `frontend-design`):**

> Repo "Pociag do Predykcji". Read `CLAUDE.md` and `services/frontend/CLAUDE.md`. Work on a `fix/`
> branch. The gateway now returns `DashboardOverview.data_freshness.operations_last_updated`
> (ISO date-time) (see `specs/openapi/gateway.yml`). The `DashboardOverview` interface in
> `src/lib/api.ts` already has it.
>
> 1. In `src/components/MapHomeClient.tsx` (live-trains section) and `src/app/pociagi/page.tsx`,
>    show "Dane z HH:mm" (Europe/Warsaw, `date-fns` + `pl` locale, already used) from
>    `operations_last_updated`. On `/pociagi`, keep the existing "Odświeżono" line but rename it to
>    make clear it is the client refresh time, or replace it with the data age. Pick one and keep
>    it minimal. `/pociagi` must use the existing `getDashboardOverview` query (react-query key
>    `['dashboardOverview']`, shared cache). Do not add a new endpoint.
> 2. If the data is older than 20 minutes, show an amber inline notice (existing `Badge`
>    variant) such as "Dane mogą być nieaktualne (ostatnia aktualizacja X min temu)". If the field
>    is missing, show nothing.
> 3. No new dependencies. Handle loading/error like the surrounding code.
>
> Acceptance: `npm run build` passes (the only gate). Check manually with the running stack:
> the notice appears when the DB snapshot is old, and the times are correct Warsaw local time.

## 5. Open questions / decisions for the user

1. **Definition of "live"**: is "status P and expected end ≥ now − 30 min, today or yesterday
   (Warsaw)" acceptable? Should trains that have not *started* yet but have status `P` be
   excluded, e.g. by requiring first stop `planned_departure <= now() + 15 min`?
2. **Refresh cadence vs PLK quota**: what is the PLK API key tier? 10 min is about 280 req/h. 5 min
   is about 560 req/h. Should the collector use `carriersInclude`/`stations` filters to shrink
   pages?
3. **Staleness policy**: if the last operations refresh is older than N minutes (e.g. 30), should
   the live endpoint return an **empty list** (strict), or return data with a warning (Step 6,
   lenient)? The proposal assumes lenient.
4. **Lake growth**: 10-minute snapshots create about 6,600 raw objects/day. Should we add a MinIO
   lifecycle rule for `raw/operations/` (e.g. 7-day expiry, devops-infra), or land only every Nth
   snapshot?
5. **Daily DAG**: once the live DAG exists, should `ingest_operations_daily` stop fetching
   operations and keep only disruptions? That would bring the DAG in line with
   `specs/airflow-dags.md` ("yesterday's completed operations"), which the code does not
   currently follow.
6. **Timestamp migration (Step 3)**: OK to run a one-shot `UPDATE` over all historical
   `operation_stations` rows, or should we instead re-process history from the lake?

## 6. Evidence queries (read-only, reproducible)

```sql
-- staleness of "in progress" today (Warsaw)
WITH last AS (
  SELECT to2.id, MAX(COALESCE(os.actual_arrival, os.planned_arrival, os.planned_departure)) AS last_ts
  FROM train_operations to2 JOIN operation_stations os ON os.train_operation_id = to2.id
  WHERE to2.train_status = 'P' AND to2.operating_date = (now() AT TIME ZONE 'Europe/Warsaw')::date
  GROUP BY 1)
SELECT count(*) total_p,
       count(*) FILTER (WHERE last_ts < now()) ended_by_plan,
       count(*) FILTER (WHERE last_ts < now() - interval '3 hours') ended_3h_ago
FROM last;                               -- 2026-09-27 15:20Z: 506 / 490 / 439

-- stored timestamps are Warsaw wall-clock labelled UTC
SELECT count(*) FILTER (WHERE (os.planned_departure AT TIME ZONE 'UTC')::date <> t.operating_date) utc_mismatch,
       count(*) FILTER (WHERE (os.planned_departure AT TIME ZONE 'Europe/Warsaw')::date <> t.operating_date) waw_mismatch
FROM operation_stations os JOIN train_operations t ON t.id = os.train_operation_id
WHERE os.planned_sequence_number = 1 AND os.planned_departure IS NOT NULL
  AND t.operating_date BETWEEN '2026-09-24' AND '2026-09-26';   -- 0 vs 876
```
