# services/go — Go services

Shared rules for `collector`, `data-service`, `gateway`. Each service has its own nested
`CLAUDE.md` with its specifics. See the repo root `CLAUDE.md` for architecture.

## Modules

- Four **separate modules**, `go 1.25.0`: `collector`, `data-service`, `gateway`, `shared`.
  Module path `github.com/pociag-do-predykcji/services/go/<name>`.
- `data-service` and `gateway` consume `shared` via a `replace ... => ../shared` directive —
  edits to `shared` are picked up with no publish step.
- Layout per service: `cmd/main.go` entrypoint; everything else under `internal/`
  (`config/`, `handler/`, `service/`, `repository/` where applicable).

## Commands (from repo root)

```bash
make collector-test        # cd services/go/collector && go test -race -v ./...
make collector-lint        # cd services/go/collector && golangci-lint run ./...
# same pattern: data-service-test / data-service-lint / gateway-test / gateway-lint
make collector-build       # swag init -g cmd/main.go -o internal/docs && go build -o /tmp/collector ./cmd

cd services/go/collector && go test -race -run TestName ./internal/service   # single test
```

- **golangci-lint must be v2** — config `services/go/<svc>/.golangci.yml` is `version: "2"`,
  `default: none`, enabled: `errcheck govet ineffassign staticcheck unused`. CI installs it with
  `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`. `shared` is not linted.
- **`swag` CLI** (`go install github.com/swaggo/swag/cmd/swag@latest`) is needed only for
  `*-build` / `*-docs`; it is not a module dependency. Not needed for test or lint.
- `shared` has **no Make targets** — `cd services/go/shared && go test -race ./...` directly.
- Integration tests use `testcontainers-go` and are gated behind the `//go:build integration`
  tag (e.g. `collector/internal/repository/repository_integration_test.go`); the default
  `go test ./...` skips them. Run with `go test -tags integration -race ./...` (needs Docker).
- `_test.go` files sit next to the code; use `t.Parallel()` where safe. Test lib: `testify`.

## Mandatory patterns

- `context.Context` is the first param of any function doing I/O.
- HTTP: `net/http` + `github.com/go-chi/chi/v5` only. No other router.
- Config: `os.LookupEnv` only, no defaults for required vars (missing → return an error from
  `config.Load`, service refuses to start). No hardcoded hosts/ports/secrets.
- Tracing: wrap every handler and DB/HTTP call with
  `otel.Tracer("pociag.<service>").Start(ctx, "<noun>.<verb>")` + `defer span.End()`.
- Errors: `fmt.Errorf("operation: %w", err)`. Never discard.
- Logging: `go.uber.org/zap`, JSON.
- Contract-first: response/request shapes must match `specs/openapi/<service>.yml`.
