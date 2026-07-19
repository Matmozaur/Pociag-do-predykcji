---
description: "Python conventions for Airflow DAGs and the pociag_processing plugin."
applyTo: "airflow/**/*.py"
---

# Python (Airflow + Plugin) Instructions

These rules apply automatically to all Python files under `airflow/`.
Python processing runs as an **Airflow plugin**, not a standalone service
(see `docs/decisions/003-processor-to-airflow-plugin.md`).

## Environment & typing

- Use `uv`-managed environments; dependencies declared in `pyproject.toml`.
- Full type annotations on every function signature. Keep `mypy --strict` clean.
- Prefer `structlog` for logging; never log secrets or raw sensitive payloads.

## DAG rules

- DAGs live in `airflow/dags/`; reusable pipeline logic lives in
  `airflow/plugins/pociag_processing/`.
- DAGs must be **idempotent** and safe to re-run for any execution date.
- Access external systems (Postgres, MinIO, PLK) via Airflow connections/hooks — never
  instantiate raw clients with inline credentials.
- Never hardcode credentials: use env vars or Airflow connections.

## Database access

- Parameterized SQL only; never concatenate untrusted values into a query.
- Keep repository/query code in `pociag_processing/repository.py`.

## Testing

- Tests live in `airflow/tests/`; run with `uv run pytest airflow/tests -v`.
- Lint/type-check with `uv run ruff check` and `uv run mypy` before finishing.

## Contract-first

Event and record shapes must match `specs/asyncapi/` and `specs/schemas/`.
Update the spec before changing a payload structure.
