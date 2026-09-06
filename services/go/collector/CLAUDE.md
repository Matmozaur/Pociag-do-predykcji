# collector

Stage-1 ingestion: pulls raw data from the PLK Open Data API and lands it as Parquet in MinIO,
recording each run in the `ingestion_run` table. Triggered by Airflow via HTTP.

Read `services/go/CLAUDE.md` for shared Go rules and commands (`make collector-test`, `-lint`).

## Specifics

- Storage client is **AWS SDK for Go v2** (`aws-sdk-go-v2/service/s3`), pointed at MinIO via
  `S3_ENDPOINT` + `S3_USE_PATH_STYLE=true`. DB access is **raw `pgx/v5`** (no ORM).
- Prometheus metrics via `prometheus/client_golang` at `GET /metrics`.
- No live Swagger endpoint — the contract is `specs/openapi/collector.yml` (see `README.md`).

## Endpoints (host `:8082`, container `:8081`)

`GET /healthz` · `GET /readyz` · `GET /metrics` · `GET /api/v1/fetch/status`
`POST /api/v1/fetch/{dictionaries|schedules|operations|disruptions}` — body like
`{"force": false}` (operations) or `{"date_from","date_to","force"}` (disruptions);
returns a `run_id`.

## Env vars (all required unless noted; `os.LookupEnv`, `config.Load` errors if missing)

`HTTP_ADDR`, `DATABASE_DSN`, `PLK_BASE_URL`, `PLK_API_KEY`, `S3_ENDPOINT`, `S3_BUCKET`,
`S3_ACCESS_KEY`, `S3_SECRET_KEY`. Optional: `S3_USE_PATH_STYLE=true`, `OTEL_EXPORTER_OTLP_ENDPOINT`.

## Tests

- `internal/plk/client_test.go`, `internal/service/service_test.go` — unit, run by default.
- `internal/repository/repository_integration_test.go` — `integration` build tag, needs Docker
  (`go test -tags integration -race ./...`).
