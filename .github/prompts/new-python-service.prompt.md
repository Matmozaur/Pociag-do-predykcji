---
mode: agent
description: Scaffold a new Python FastAPI microservice following project conventions
---

> **Before running this prompt**: ensure a spec exists at
> `specs/openapi/${input:serviceName}.yml`. If not, run `#design-api-spec` first.

Scaffold a new Python microservice named **`${input:serviceName}`** under `services/python/${input:serviceName}/`.
The service must implement exactly the operations defined in `specs/openapi/${input:serviceName}.yml` —
do not add endpoints not present in the spec.

## Requirements

Create the following structure:

```
services/python/${input:serviceName}/
├── src/
│   └── ${input:serviceName}/
│       ├── __init__.py
│       ├── main.py              ← FastAPI app factory, OTel setup, lifespan
│       ├── config.py            ← pydantic-settings BaseSettings
│       ├── dependencies.py      ← FastAPI Depends providers (db, tracer)
│       ├── router.py            ← API routes
│       ├── models.py            ← Pydantic request/response models
│       └── repository.py        ← asyncpg raw SQL queries
├── tests/
│   ├── conftest.py              ← pytest fixtures, testcontainers Postgres
│   └── test_router.py           ← endpoint tests with httpx AsyncClient
├── pyproject.toml               ← uv-compatible, PEP 621
├── Dockerfile                   ← multi-stage, non-root user
└── .instructions.md             ← Copilot scoped instructions for this service
```

## Conventions to follow

- `config.py`: `class Settings(BaseSettings)` with `model_config = SettingsConfigDict(env_file=".env")`.
- `main.py`: use `@asynccontextmanager` lifespan; instrument with `FastAPIInstrumentor`; add `/healthz` and `/readyz` routes.
- `repository.py`: accept `asyncpg.Connection`; use parameterized `$1, $2, ...` placeholders; return typed dataclasses or Pydantic models.
- `router.py`: annotate all path operations with response models; use `Annotated` for dependency injection.
- `pyproject.toml`: include `fastapi`, `asyncpg`, `pydantic-settings`, `structlog`, `opentelemetry-sdk`, `opentelemetry-instrumentation-fastapi`.
- `Dockerfile`: `FROM python:3.12-slim AS builder` with uv; `FROM python:3.12-slim`; run as `UID 1001`.

Generate complete, type-annotated, production-ready Python code.
