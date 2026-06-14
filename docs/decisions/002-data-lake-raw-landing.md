# ADR-002: Data Lake Raw Landing — Collector writes Parquet to MinIO (S3)

## Status

Accepted

## Date

2026-06-14

## Context

ADR-001 established a two-stage ELT pipeline where the Collector writes raw PLK API
payloads as JSONB into PostgreSQL `raw_*` tables. While functional, this approach has
downsides as data volume grows:

1. **Storage cost** — Storing large JSON blobs in PostgreSQL is expensive (WAL, VACUUM, bloat).
2. **Scalability** — PostgreSQL is not designed as a bulk raw data store; Parquet is 5–10x
   more storage-efficient for columnar analytical reads.
3. **Processing flexibility** — The Processor (Python/polars) benefits from reading Parquet
   natively rather than streaming JSONB from PostgreSQL.
4. **Separation of concerns** — Raw landing should be cheap, append-only, immutable storage.
   PostgreSQL should only hold curated relational data optimized for queries.
5. **Future ML pipelines** — Feature engineering and model training naturally consume Parquet
   from object storage, not database BLOBs.

## Decision

Replace PostgreSQL `raw_*` tables with **MinIO (S3-compatible object storage)** as the raw
landing zone. The Collector writes **Parquet files** to MinIO. The Processor reads Parquet
from MinIO and writes curated data to PostgreSQL.

### Architecture Change

```
┌──────────┐         ┌───────────┐  Parquet     ┌──────────────┐
│ PLK Open │◄────────│ Collector │─────────────►│    MinIO      │
│ Data API │  fetch  │   (Go)    │              │ (S3-compat)  │
└──────────┘         └───────────┘              │ pociag-lake/ │
                           │                    └──────┬───────┘
                           │ tracks run status         │ reads Parquet
                           ▼                           ▼
                     ┌──────────────┐          ┌───────────┐
                     │  PostgreSQL  │          │ Processor │◄── Airflow triggers
                     │ingestion_runs│          │ (Python)  │
                     └──────────────┘          └─────┬─────┘
                                                     │ writes curated
                                                     ▼
                                               ┌──────────────┐
                                               │  PostgreSQL  │
                                               │ domain tables│
                                               └──────────────┘
```

### Storage Layout

```
s3://pociag-lake/
  raw/
    dictionaries/
      YYYY/MM/DD/
        run_<id>_<type>.parquet       # e.g. run_42_stations.parquet
    schedules/
      YYYY/MM/DD/
        run_<id>_page_<N>.parquet
    operations/
      YYYY/MM/DD/
        run_<id>_page_<N>.parquet
    disruptions/
      YYYY/MM/DD/
        run_<id>.parquet
```

- Partitioned by date for efficient range scans.
- `run_<id>` ties each file to the `ingestion_runs` table for lineage.
- Parquet files are immutable once written (append-only semantics).

### Collector Changes

| Before | After |
|---|---|
| `pgx/v5` writes to `raw_*` tables | `aws-sdk-go-v2` PutObject to MinIO + `apache/arrow-go` Parquet writer |
| `DATABASE_DSN` for raw landing | `S3_ENDPOINT`, `S3_BUCKET`, `S3_ACCESS_KEY`, `S3_SECRET_KEY` |
| Still uses PostgreSQL for `ingestion_runs` tracking | Same — run tracking stays in PostgreSQL |

The Collector **still connects to PostgreSQL** for `ingestion_runs` (run status, conflict
detection, metadata) but no longer writes raw payloads there.

### Processor Changes

| Before | After |
|---|---|
| Reads `raw_*` tables via `asyncpg` SQL | Reads Parquet from MinIO via `s3fs` + `polars` |
| Queries filtered by `ingestion_run_id` | Reads files by path pattern: `raw/<pipeline>/<date>/run_<id>*` |

### What Stays the Same

- Collector API contract (OpenAPI spec) — same endpoints, same request/response schemas
- Airflow DAGs — same task structure (fetch → process)
- Curated PostgreSQL domain tables — unchanged
- `ingestion_runs` tracking table — stays in PostgreSQL
- Data Service, Gateway — no changes (they read curated tables only)

### MinIO Configuration (Local Dev)

- Container: `minio/minio`
- Ports: 9000 (API), 9001 (console UI)
- Default bucket: `pociag-lake` (created on startup)
- Credentials: `minioadmin` / `minioadmin` (local dev only)

## Trade-offs

| Alternative | Pros | Cons | Verdict |
|---|---|---|---|
| **MinIO + Parquet** | Scalable, columnar, S3-compatible, ML-friendly | Extra container, new SDK | **Accepted** |
| Keep PostgreSQL JSONB | Already implemented, simple | Expensive storage, poor analytical perf | Rejected |
| Local filesystem Parquet | No extra container | Not production-like, no S3 API | Rejected |
| Delta Lake / Iceberg | ACID, time travel | Overkill for append-only raw landing | Future option |

## Consequences

### Positive

- 5–10x storage efficiency for raw data (Parquet columnar compression)
- Processor reads are faster (native Parquet → polars, no SQL parsing overhead)
- Clean separation: PostgreSQL = relational queries only, MinIO = bulk raw storage
- ML pipelines can read training data directly from the lake
- Raw data retention is cheap (object lifecycle policies)
- Production path is clear (MinIO → AWS S3 / GCS with zero code changes)

### Negative

- New infrastructure dependency (MinIO container)
- Collector needs Parquet + S3 libraries (increases binary size ~5MB)
- Slightly more complex debugging (can't just `SELECT * FROM raw_*`)
- Two storage systems to manage in dev (PostgreSQL + MinIO)

### Migration Path

1. Add MinIO to `docker-compose.yml`
2. Add `internal/lake/` package to Collector (S3 client + Parquet writer)
3. Refactor `internal/repository/` — remove `InsertRaw*` methods, keep `ingestion_runs`
4. Update Processor to read from S3 instead of `raw_*` tables
5. Write down migration `007_drop_raw_tables.down.sql` / `.up.sql` (drops `raw_*` tables)
6. Remove old `raw_*` migration files or mark deprecated

### Database Migrations Affected

- `000_raw_landing.up.sql` — will be superseded (tables no longer needed)
- New migration: drop `raw_*` tables (additive: create a new numbered migration)
- `005_ingestion_tracking.up.sql` — unchanged (still used for run metadata)

## References

- [ADR-001: Base Platform Architecture](001-base-platform-architecture.md)
- [Collector OpenAPI spec](../../specs/openapi/collector.yml)
- [Processor OpenAPI spec](../../specs/openapi/processor.yml)
- [MinIO Docker docs](https://min.io/docs/minio/container/index.html)
- [apache/arrow-go Parquet](https://github.com/apache/arrow-go)
