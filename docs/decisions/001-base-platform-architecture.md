# ADR-001: Base Platform Architecture — ELT Pipeline + Microservices + BFF

## Status

Accepted

## Date

2025-05-24

## Context

We are building a train monitoring and prediction platform that ingests data from the
PLK Open Data API (schedules, real-time operations, disruptions) and presents it to
users via a web frontend. The platform must:

1. Periodically ingest schedule data (weekly, 2-week horizon)
2. Daily pull historical operations and disruptions for analysis
3. Provide a fast read API for the frontend
4. Be extensible for future features: live tracking, delay prediction, maps, notifications
5. Allow rich data processing (deduplication, normalization, enrichment) before serving

Key constraints:
- PLK API has rate limits (100-2000 req/hour)
- Data quality varies (shortened fields, nullable values, potential duplicates)
- Read patterns differ significantly from write/processing patterns
- Must support future ML inference pipeline without architectural rewrites
- Frontend needs aggregated views that don't map 1:1 to domain tables

## Decision

Adopt a **microservice architecture** with:
- **ELT pipeline** (Extract → Land → Transform) for data ingestion
- **CQRS** for write/read separation
- **BFF (Backend for Frontend)** pattern for the Gateway

### Service Boundaries

| Service | Language | Responsibility | Port |
|---|---|---|---|
| **Collector** | Go | Extract raw data from PLK API → raw landing tables | 8081 |
| **Processor** | Python | Transform raw → curated domain tables (dedup, normalize, enrich) | 8082 |
| **Data Service** | Go | Owns curated domain data, reads PostgreSQL directly, exposes fine-grained REST APIs | 8083 |
| **Gateway (BFF)** | Go | Frontend-facing facade, aggregates Data Service calls, shapes responses; no direct DB access | 8080 |
| **Airflow** | Python | Orchestrates Collector → Processor pipeline | 8090 |
| **Frontend** | TBD | Web UI for schedule search, extensible | 3001 |

### Data Flow

```
┌──────────┐         ┌───────────┐  raw JSONB   ┌──────────────┐
│ PLK Open │◄────────│ Collector │─────────────►│  PostgreSQL  │
│ Data API │  fetch  │   (Go)    │              │  raw_* tables│
└──────────┘         └───────────┘              └──────┬───────┘
                                                       │ reads raw
                     ┌───────────┐                     ▼
                     │ Processor │◄────────── Airflow triggers
                     │ (Python)  │
                     └─────┬─────┘
                           │ writes curated
                           ▼
                     ┌──────────────┐
                     │  PostgreSQL  │
                     │ domain tables│
                     └──────┬───────┘
                            │ SQL (reads)
                            ▼
                     ┌──────────┐
                     │   Data   │
                     │ Service  │
                     └────┬─────┘
                          │ HTTP
                          ▼
┌──────────┐  HTTP   ┌─────────────┐
│ Frontend │◄───────►│   Gateway   │
│  (Web)   │         │   (BFF)     │
└──────────┘         └─────────────┘
```

### Data Pipeline Stages

Instead of traditional bronze/silver/gold (designed for data lakes), we use a
**Raw → Curated** approach optimized for PostgreSQL:

| Stage | Storage | Owner | Description |
|---|---|---|---|
| **Raw (Landing)** | `raw_*` tables (JSONB) | Collector | Exact PLK API responses, timestamped, immutable append |
| **Curated (Domain)** | Normalized relational tables | Processor | Deduplicated, validated, enriched relational data |

**Why Raw → Curated over bronze/silver/gold?**
- PostgreSQL, not a data lake — no need for Parquet/Delta layers
- Two stages are sufficient: raw preserves source-of-truth, curated serves queries
- Adding a "gold" aggregation layer later (materialized views) is trivial
- Raw tables enable full reprocessing if Processor logic changes

**Why Python for processing?**
- Rich data manipulation capabilities (polars for performance)
- Natural fit for future ML feature engineering pipeline
- Easier to express complex dedup/normalization logic than in Go
- Same language as the future Predictor service — shared transformation utils

### BFF Pattern (Gateway)

The Gateway does **NOT** access the database directly. It:
1. Aggregates multiple Data Service calls into frontend-optimized responses
2. Handles response shaping (field selection, nesting, localization)
3. Implements frontend-specific concerns (pagination UX, search suggestions)
4. Acts as the single entry point for all frontend needs
5. Can add caching layer (Redis) without affecting domain services
6. Will later aggregate Predictor, Notification, and other services

### Data Service Ownership

The Data Service is the only read API service that accesses curated PostgreSQL domain tables directly.
It does not rely on Gateway for any database operations. Gateway is a consumer of Data Service APIs,
not a data-access dependency.

### Trade-off Analysis

**Architecture style:**

| Alternative | Pros | Cons | Verdict |
|---|---|---|---|
| Monolith | Simple | Coupling, harder to scale, harder to add ML | Rejected |
| **Microservices + BFF** | Independent scaling, clear boundaries, extensible | More services | **Accepted** |
| Event Sourcing | Full audit trail | Over-engineered for v1 | Future option |

**BFF vs direct service exposure:**

| Alternative | Pros | Cons | Verdict |
|---|---|---|---|
| FE → Data Service directly | Simple | Tight coupling, N+1 calls, FE logic leaks into service | Rejected |
| **FE → Gateway (BFF) → Data Service** | Decoupled, FE-optimized, single entry point | Extra hop | **Accepted** |
| GraphQL gateway | Flexible queries | Complexity, cache difficulty, security surface | Future option |

**Processing language:**

| Alternative | Pros | Cons | Verdict |
|---|---|---|---|
| Go for everything | Single language | Verbose data transforms, no ML ecosystem | Rejected |
| **Go extract + Python transform** | Best of both: fast I/O + rich processing | Two languages | **Accepted** |
| Python for everything | Simple | Slower extraction, higher memory for HTTP services | Rejected |

**Orchestration:**

| Alternative | Pros | Cons | Verdict |
|---|---|---|---|
| Cron + scripts | Simple | No retry, no monitoring, no DAG deps | Rejected |
| **Airflow** | DAG viz, retry, backfill, sensor tasks, Python native | Heavier infra | **Accepted** |
| Temporal | Better for long-running workflows | Overkill for batch ELT | Future option |

## Consequences

### Positive

- Clear service boundaries: each service has a single responsibility
- Python Processor enables rich data transformation + future ML pipeline reuse
- Raw tables preserve PLK source data for reprocessing/debugging
- BFF isolates frontend from domain model changes
- Data Service can be split later (schedule-service, operations-service) if needed
- Each service is independently deployable and scalable
- Adding new FE features = new BFF endpoint, not new domain service changes

### Negative

- 4 services to build and deploy (vs 2 in CQRS-only approach)
- Extra network hop for reads (FE → Gateway → Data Service → DB)
- Raw tables consume additional storage (mitigated by JSONB compression + retention)
- Python Processor adds dependency on Python runtime in the pipeline

### Risks

- PLK API rate limits may require throttling logic in Collector
- Raw JSONB tables will grow; need retention policy (keep 90 days of raw)
- Network latency Gateway ↔ Data Service (mitigated by co-location + connection pooling)
- Processor bugs could corrupt curated data (mitigated by raw tables as reprocessing source)
- Service mesh complexity at scale (mitigated: start with Docker Compose, add K8s later)

## References

- [PLK Open Data API spec](../specs/openapi/plk-open-data.json)
- [Gateway (BFF) spec](../specs/openapi/gateway.yml)
- [Collector spec](../specs/openapi/collector.yml)
- [Processor spec](../specs/openapi/processor.yml)
- [Data Service spec](../specs/openapi/data-service.yml)
- [Platform Events](../specs/asyncapi/platform-events.yml)
- [Airflow DAG specs](../specs/airflow-dags.md)
- [DB Migrations](../db/migrations/)
