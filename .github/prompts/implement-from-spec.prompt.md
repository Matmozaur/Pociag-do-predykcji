---
mode: agent
description: Implement a service from an OpenAPI spec
---

Implement code from the spec at **`${input:specFile}`** (e.g. `specs/openapi/my-service.yml`).

## Steps

1. **Read the spec** — parse all paths, operations, schemas, and parameters from the file.
2. **Identify the target** — `specs/openapi/api.yml` is served by the Go api (`services/go/api/`);
   `specs/pipelines.md` is implemented by the Airflow DAGs and `pociag_processing` plugin
   (`airflow/`). For any other spec, ask where it should live.
3. **Generate only what the spec defines** — do not add endpoints, fields, or behaviours
   not present in the spec. If the spec is incomplete, flag the gap rather than guessing.

## Mapping rules

### OpenAPI → Go api (`services/go/api/`)

| OpenAPI element | Go code |
|---|---|
| `paths` | Routes in `Handler.Routes` + an unexported camelCase method (e.g. `h.stationBoard`) in `internal/handler/handler.go` |
| `components/schemas` | Response models in `internal/model/` |
| Data from PostgreSQL | `internal/repository/` query methods + `internal/service/` view building |

Follow Go conventions from `.github/copilot-instructions.md`:
- Context as first param, OTel span per handler and DB call, parameterized SQL, zap logging.

### Pipeline spec → Airflow (`airflow/`)

Follow `airflow/CLAUDE.md`: thin DAG in `dags/`, logic in `plugins/pociag_processing/`
(`transform.py` for PLK payloads, SQL only in `repository.py`), idempotent upserts,
`mypy --strict`.

## Output

Generate only the code needed to satisfy the spec exactly.
Include a brief summary of what was created and any spec gaps found.
