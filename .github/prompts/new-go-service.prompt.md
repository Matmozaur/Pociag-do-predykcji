---
mode: agent
description: Scaffold a new Go microservice following project conventions
---

> **Before running this prompt**: ensure a spec exists at
> `specs/openapi/${input:serviceName}.yml`. If not, run `#design-api-spec` first.

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
│   │   └── handler.go           ← HTTP handlers, OTel-instrumented
│   ├── repository/
│   │   └── repository.go        ← pgx/v5 DB queries
│   └── service/
│       └── service.go           ← business logic, calls repository
├── go.mod                       ← module: github.com/pociag-do-predykcji/services/go/${input:serviceName}
├── Dockerfile                   ← multi-stage build, non-root user
├── .golangci.yml                ← golangci-lint config
└── .instructions.md             ← Copilot scoped instructions for this service
```

## Conventions to follow

- `config.go`: struct with fields populated via `os.LookupEnv`; return error if required vars are missing.
- `handler.go`: inject `service` and `tracer`; every handler starts an OTel span; return JSON responses.
- `repository.go`: use `pgx/v5` pool passed via constructor; parameterized queries only.
- `service.go`: pure business logic, no HTTP or DB imports.
- `main.go`: initialize OTel SDK with OTLP exporter; register `/healthz`, `/readyz`, `/metrics` endpoints; graceful shutdown on SIGTERM/SIGINT.
- Dockerfile: `FROM golang:1.23-alpine AS builder` → `FROM alpine:3.21`; run as `UID 1001`.
- Add a `TestMain` in each `_test` package to set up testcontainers-based Postgres.

Generate idiomatic, production-ready Go code with all imports and error handling.
