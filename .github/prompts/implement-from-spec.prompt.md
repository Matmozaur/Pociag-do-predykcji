---
mode: agent
description: Implement a service from an OpenAPI or AsyncAPI spec
---

Implement code from the spec at **`${input:specFile}`** (e.g. `specs/openapi/my-service.yml`).

## Steps

1. **Read the spec** — parse all paths, operations, schemas, and parameters from the file.
2. **Identify the service language** — check `services/go/` or `services/python/` for an existing
   service matching the spec name. If none exists, ask which language to use.
3. **Generate only what the spec defines** — do not add endpoints, fields, or behaviours
   not present in the spec. If the spec is incomplete, flag the gap rather than guessing.

## Mapping rules

### OpenAPI → Go service (`services/go/<name>/`)

| OpenAPI element | Go code |
|---|---|
| `operationId` | Handler method name: `Handle<OperationId>` |
| `paths` | Routes in `internal/handler/handler.go` |
| `components/schemas` | Struct types in `internal/handler/handler.go` (request/response) |
| `components/schemas` (domain) | Struct types in `internal/repository/repository.go` |
| DB persistence implied | `internal/repository/` methods + `internal/service/` logic |

Follow Go conventions from `.github/copilot-instructions.md`:
- Context as first param, OTel span per handler and DB call, parameterized SQL, zap logging.

### OpenAPI → Python service (`services/python/<name>/`)

| OpenAPI element | Python code |
|---|---|
| `operationId` | FastAPI path function name (snake_case) |
| `paths` | Routes in `src/<name>/router.py` |
| `components/schemas` | Pydantic models in `src/<name>/models.py` |
| DB persistence implied | `src/<name>/repository.py` + asyncpg |

Follow Python conventions from `.github/copilot-instructions.md`:
- `async def` everywhere, type annotations, OTel span per endpoint and DB call, structlog.

### AsyncAPI → DAG or consumer

- One channel → one Airflow task or consumer function.
- `send` operation → Airflow task that publishes the message.
- `receive` operation → Airflow task or service endpoint that processes the message.

## Output

Generate all files needed for a runnable service that exactly satisfies the spec.
Include a brief summary of what was created and any spec gaps found.
