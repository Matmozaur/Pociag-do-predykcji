# ADR 003: Migrate Processor Service to Airflow Plugin

## Status

Accepted

## Context

The processor service (`services/python/processor`) was a standalone FastAPI microservice
whose sole consumers were Airflow DAGs calling it via HTTP. This added:

- An extra deployment unit (Docker container, health checks, port management)
- Network latency between Airflow tasks and processing logic
- Credential duplication (DB and S3 creds in both Airflow and the processor)
- Complexity in connection management (`asyncpg` pool lifecycle, HTTP timeouts)

The processor had 4 pipelines: dictionaries, schedules, operations, and disruptions.
All read raw JSON from S3/MinIO (data lake), transform, and upsert into PostgreSQL.

## Decision

Migrate all processing logic into `airflow/plugins/pociag_processing/` as a native
Python package callable directly from Airflow tasks. Specifically:

1. **Replace asyncpg with psycopg2** via Airflow's built-in `PostgresHook` — natural fit
   for Airflow's synchronous task execution model.
2. **Use Airflow Connections** (`pociag_postgres`, `pociag_s3`) for all credential management.
3. **Structure as plugin package** with `pipelines/` subpackage exposing one function per
   pipeline (e.g., `process_dictionaries(run_date, ingestion_run_id)`).
4. **Update existing DAGs** to import and call plugin functions directly instead of making
   HTTP calls to the processor service.
5. **Delete the standalone processor service** and its OpenAPI spec.
6. **Preserve OpenTelemetry tracing** via SDK initialization in the plugin's `tracing.py`.

## Alternatives Considered

| Alternative | Pros | Cons |
|-------------|------|------|
| Keep processor as HTTP service | No code changes; clear service boundary | Extra container; network hop; credential duplication; asyncpg complexity in sync context |
| Wrap asyncpg with `asyncio.run()` in Airflow tasks | Reuse existing async code | Fragile nested event-loop issues in some Airflow executors; adds complexity |
| Use psycopg 3 (async-capable, sync default) | Modern driver; sync by default | New dependency; slight learning curve; no Airflow hook integration |

## Consequences

### Positive

- Fewer services to deploy, monitor, and maintain
- Simpler DAG logic — no HTTP client management, timeouts, or connection lookups
- Native Airflow retry and alerting on processing failures
- All credentials managed in one place (Airflow Connections)
- Reduced latency — no network hop between DAG task and processing

### Negative

- Processing logic now coupled to the Airflow runtime — cannot be invoked independently via HTTP
  (acceptable: no external consumers exist)
- Plugin code must be compatible with Airflow worker Python environment

### Neutral

- OpenTelemetry tracing preserved via SDK initialization in plugin
- Run tracking (`ingestion_runs` table) continues to work as before
- DB schema unchanged — no migration needed
