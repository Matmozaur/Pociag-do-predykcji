# services/go — Go

One module: `api` (`github.com/pociag-do-predykcji/services/go/api`, `go 1.25.0`). See the repo
root `CLAUDE.md` for architecture and `specs/openapi/api.yml` for the contract.

## api layout

- `cmd/main.go` — config, tracing, pgx pool, HTTP server.
- `internal/handler` — chi routes, parameter validation (out of range → 400), JSON, error
  mapping (`service.ErrNotFound` → 404, else logged 500).
- `internal/service` — builds the response models (`internal/model`) from repository rows. Pure
  apart from the `Repository` interface it defines; tests use `service/servicetest` (fake repo)
  and `export_test.go` (fixed clock).
- `internal/repository` — all SQL (pgx v5, no ORM), one method per view.
- `internal/position` — pure train-position model (active predicate, phase, interpolation).

## Commands (from repo root)

```bash
make api-test     # cd services/go/api && go test -race ./...
make api-lint     # golangci-lint v2 (.golangci.yml: errcheck govet ineffassign staticcheck unused)
POCIAG_TEST_DATABASE_URL=postgres://… make api-test   # also runs internal/repository SQL tests
```

## Env vars

`HTTP_ADDR`, `DATABASE_DSN` (required); `OTEL_EXPORTER_OTLP_ENDPOINT` (optional, tracing off when
unset).

## Patterns

- `context.Context` first on I/O; errors wrapped `fmt.Errorf("op: %w", err)`.
- `net/http` + `chi/v5` only; `zap` JSON logs; `otelhttp` wraps the router, repository methods
  open `db.<table>.<verb>` spans.
- Times shown to people are formatted Europe/Warsaw `HH:MM` in `service`; `time/tzdata` is
  embedded because the runtime image has no zoneinfo.
- Response shapes must match `specs/openapi/api.yml` and `services/frontend/src/lib/api.ts`.
