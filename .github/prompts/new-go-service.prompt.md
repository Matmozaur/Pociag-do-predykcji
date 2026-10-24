---
mode: agent
description: Scaffold a new Go microservice following project conventions
---

> **Before running this prompt**: ensure a spec exists at
> `specs/openapi/${input:serviceName}.yml`. If not, run `#design-api-spec` first.
>
> The project deliberately has **one** read service (`services/go/api`, see
> `docs/architecture.md`). New frontend endpoints belong there; confirm with the user that a
> separate service is really wanted before scaffolding one. Mirror `services/go/api`'s layout.

Scaffold a new Go microservice named **`${input:serviceName}`** under `services/go/${input:serviceName}/`.
The service must implement exactly the operations defined in `specs/openapi/${input:serviceName}.yml` —
do not add endpoints not present in the spec.

## Requirements

Create the following structure:

```
services/go/${input:serviceName}/
├── cmd/
│   └── main.go                  ← entrypoint: reads config, wires deps, starts HTTP server
├── internal/
│   ├── config/
│   │   └── config.go            ← env-var config struct with validation
│   ├── handler/
│   │   └── handler.go           ← chi routes, validation, JSON, error mapping
│   ├── model/
│   │   └── model.go             ← response types matching the spec
│   ├── repository/
│   │   └── repository.go        ← pgx/v5 DB queries, one span per method
│   └── service/
│       └── service.go           ← builds responses; defines the Repository interface it needs
├── go.mod                       ← module: github.com/pociag-do-predykcji/services/go/${input:serviceName}, go 1.25.0
├── Dockerfile                   ← multi-stage build, non-root user
└── .golangci.yml                ← golangci-lint v2 config (copy services/go/api's)
```

## Conventions to follow

- `config.go`: struct with fields populated via `os.LookupEnv`; return error if required vars are missing.
- `handler.go`: inject `*service.Service` and `*zap.Logger`; `Routes()` returns the chi router with `/healthz` and `/readyz`; return JSON responses.
- `repository.go`: use `pgx/v5` pool passed via constructor; parameterized queries only; each method opens a `db.<table>.<verb>` span.
- `service.go`: pure business logic, no HTTP or DB imports; tests use a fake repository package (`service/servicetest`).
- `main.go`: zap production logger; initialize OTel SDK with OTLP gRPC exporter when `OTEL_EXPORTER_OTLP_ENDPOINT` is set; wrap the router in `otelhttp`; graceful shutdown on SIGTERM/SIGINT; import `time/tzdata`.
- Dockerfile: `FROM golang:1.25.0-alpine3.22 AS builder` → `FROM alpine:3.22.1`; run as `UID 1001`.
- SQL tests run against a disposable Postgres from `POCIAG_TEST_DATABASE_URL` and skip when it is unset (no testcontainers).
- Add the service to `infra/docker-compose.yml`, the `Makefile` checks and `.github/workflows/ci-cd.yaml`.

Generate idiomatic, production-ready Go code with all imports and error handling.
