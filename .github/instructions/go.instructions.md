---
description: "Go service conventions for collector, data-service, and gateway."
applyTo: "services/go/**/*.go"
---

# Go Instructions

These rules apply automatically to all Go files in `services/go/`.

## Structure

- Module path: `github.com/pociag-do-predykcji/services/go/<service>`.
- Entrypoints in `cmd/`; all non-exported code in `internal/`.
- Shared helpers live in `services/go/shared/`.

## Mandatory patterns

- `context.Context` is **always** the first parameter of any function performing I/O.
- HTTP routing: `net/http` + `chi`. No other router frameworks.
- Database: `pgx/v5` with **parameterized queries only** — never string-concatenate SQL.
- Tracing: wrap every handler and DB call with
  `otel.Tracer("pociag.<service>").Start(ctx, "<noun>.<verb>")` and `defer span.End()`.
- Errors: wrap with `fmt.Errorf("operation: %w", err)`. Never swallow or discard errors.
- Config: read from `os.LookupEnv` only. No hardcoded hosts, ports, or secrets.

## Testing

- `_test.go` files live alongside production code.
- Unit tests must call `t.Parallel()` where safe.
- DB integration tests use `testcontainers-go` and are gated behind the `integration` build tag.
- Run `go test -race ./...` and `golangci-lint run ./...` before considering work done.

## Contract-first

Do not add endpoints, fields, or response shapes that are not defined in
`specs/openapi/`. If a contract change is needed, update the spec first, then the code.
