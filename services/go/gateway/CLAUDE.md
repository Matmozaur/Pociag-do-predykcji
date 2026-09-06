# gateway

Frontend-facing BFF. Aggregates/reshapes data-service responses; owns CORS, response caching,
and rate limiting. **No database** and no direct PLK/collector access.

Read `services/go/CLAUDE.md` for shared Go rules and commands (`make gateway-test`, `-lint`).

## Structure

- `internal/client/dataservice/` — typed HTTP client for data-service (`client.go`, `types.go`).
- `internal/mapper/` — data-service DTOs → gateway response models (`internal/model/gateway.go`).
- `internal/middleware/` — `cache.go`, `cors.go`, `ratelimit.go` (token bucket via
  `golang.org/x/time/rate`). Keep new cross-cutting HTTP concerns here as chi middleware.
- Response shapes are the contract in `specs/openapi/gateway.yml`; the frontend depends on them.

## Env vars

Required: `HTTP_ADDR`, `DATA_SERVICE_BASE_URL` (`config.Load` errors if unset).
Optional: `OTEL_EXPORTER_OTLP_ENDPOINT` (+ other tunables read via helper `lookup*` funcs in
`internal/config/config.go` — check there before adding a new one).

## Tests

`internal/handler/handler_test.go`, `internal/service/service_test.go`,
`internal/middleware/ratelimit_test.go` — all unit, `make gateway-test`.
