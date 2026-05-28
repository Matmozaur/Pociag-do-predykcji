# Airflow DAG Specifications

This document specifies the DAGs for the Pociag do Predykcji data ingestion pipelines.
Each DAG uses a two-stage workflow:
1. Collector fetches PLK payloads into raw landing tables (bronze)
2. Processor transforms and upserts curated domain tables (silver)

---

## DAG 1: `sync_dictionaries_weekly`

| Property | Value |
|---|---|
| Schedule | `0 3 * * 1` (Monday 03:00 UTC) |
| Catchup | `False` |
| Retries | 2, delay 5min |
| Purpose | Sync reference data (stations, carriers, categories, stop types) |

### Tasks

1. **fetch_dictionaries** — `POST http://collector:8081/api/v1/fetch/dictionaries`
   - Request body:
   ```json
   {
     "force": false
   }
   ```
   - Expected: 200 with fetch result

2. **process_dictionaries** — `POST http://processor:8082/api/v1/process/dictionaries`
   - Triggered only if fetch succeeded
   - Expected: 200 with process result
   - On failure: retry + alert

### Logic

- Dictionary data changes rarely; weekly sync is sufficient.
- First run on deployment will populate all dictionaries.

### Data Flow

```
PLK /dictionaries/* -> Collector /fetch/dictionaries -> raw_dictionaries
raw_dictionaries -> Processor /process/dictionaries -> carriers, stations, commercial_categories, stop_types
```

---

## DAG 2: `ingest_schedules_weekly`

| Property | Value |
|---|---|
| Schedule | `0 4 * * 1` (Monday 04:00 UTC) |
| Catchup | `False` |
| Retries | 2, delay 5min |
| Purpose | Pull train schedules for the next 2 weeks |

### Tasks

1. **check_last_run** — Query `GET http://processor:8082/api/v1/process/status?pipeline=schedules&limit=1`
   - If last successful run < 7 days ago → skip (short-circuit)
   - If no recent run OR last run > 7 days → proceed

2. **fetch_schedules** — `POST http://collector:8081/api/v1/fetch/schedules`
   ```json
   {
     "date_from": "{{ today }}",
     "date_to": "{{ today + 14 days }}",
     "force": false
   }
   ```
   - Collector lands raw PLK pages into `raw_schedules`

3. **process_schedules** — `POST http://processor:8082/api/v1/process/schedules`
   ```json
   {
       "date_from": "{{ today }}",
       "date_to": "{{ today + 14 days }}"
   }
   ```
    - Processor deduplicates and upserts curated schedule data

4. **verify_result** — Check response status and records_written

### Immediate-Run Logic

On DAG first enable (or manual trigger), if `check_last_run` finds no successful
run in the past 7 days, the DAG proceeds immediately regardless of schedule.

### Deduplication

- Collector stores raw payload snapshots.
- Processor applies idempotent upsert rules to curated datasets.

---

## DAG 3: `ingest_operations_daily`

| Property | Value |
|---|---|
| Schedule | `0 2 * * *` (Daily 02:00 UTC) |
| Catchup | `False` |
| Retries | 2, delay 5min |
| Purpose | Pull yesterday's completed operations for historical analysis |

### Tasks

1. **fetch_operations** — `POST http://collector:8081/api/v1/fetch/operations`
   ```json
   {
     "date": "{{ yesterday }}",
     "force": false
   }
   ```
   - Pulls all pages from PLK operations endpoint
   - Lands raw payloads into `raw_operations`

2. **process_operations** — `POST http://processor:8082/api/v1/process/operations`
   ```json
   {
       "date": "{{ yesterday }}"
   }
   ```
    - Upserts curated operations data

3. **fetch_disruptions** — `POST http://collector:8081/api/v1/fetch/disruptions`
   ```json
   {
     "date_from": "{{ yesterday }}",
     "date_to": "{{ yesterday }}",
     "force": false
   }
   ```
   - Lands raw payloads into `raw_disruptions`

4. **process_disruptions** — `POST http://processor:8082/api/v1/process/disruptions`
   ```json
   {
       "date_from": "{{ yesterday }}",
       "date_to": "{{ yesterday }}"
   }
   ```
    - Upserts curated disruptions data

5. **log_statistics** — Log operation statistics for monitoring
   - Emit metrics: total trains, delays, cancellations

### Data Flow

```
PLK /operations -> Collector /fetch/operations -> raw_operations
raw_operations -> Processor /process/operations -> curated operations

PLK /disruptions -> Collector /fetch/disruptions -> raw_disruptions
raw_disruptions -> Processor /process/disruptions -> curated disruptions
```

---

## DAG 4: `ingest_operations_catchup`

| Property | Value |
|---|---|
| Schedule | `None` (manual trigger only) |
| Catchup | `False` |
| Purpose | Backfill historical operations for a date range |

### Parameters (DAG params)

- `start_date`: First date to ingest (format: YYYY-MM-DD)
- `end_date`: Last date to ingest (format: YYYY-MM-DD)

### Tasks

1. **generate_dates** — Produce list of dates in range
2. **fetch_operations_batch** — For each date, call `POST /api/v1/fetch/operations`
   - Uses dynamic task mapping
   - Respects PLK rate limits (sequential with delay)
3. **process_operations_batch** — For each date, call `POST /api/v1/process/operations`
4. **fetch_disruptions_batch** — For each date, call `POST /api/v1/fetch/disruptions`
5. **process_disruptions_batch** — For each date, call `POST /api/v1/process/disruptions`

---

## Shared Conventions

- All DAGs use the `@dag` decorator (TaskFlow API)
- HTTP calls use `HttpOperator` or `@task` with `httpx`
- Collector base URL from Airflow Connection: `pociag_collector`
- Processor base URL from Airflow Connection: `pociag_processor`
- All DAGs tagged with `["pociag", "ingestion"]`
- Failure alerts via Airflow email/Slack (configurable)
- Each task is idempotent — safe to re-run

## Rate Limit Awareness

PLK API limits: 100-2000 req/hour depending on tier.
The Collector service handles pagination internally and respects rate limits
with exponential backoff. DAGs should avoid parallel fetch calls to prevent throttling.
