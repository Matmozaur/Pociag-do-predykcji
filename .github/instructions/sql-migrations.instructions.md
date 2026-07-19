---
description: "Database migration conventions (golang-migrate)."
applyTo: "db/migrations/**/*.sql"
---

# SQL Migration Instructions

These rules apply automatically to all files in `db/migrations/`.

## Naming & pairing

- Every migration is a pair: `NNN_description.up.sql` and `NNN_description.down.sql`.
- `NNN` is a zero-padded, monotonically increasing sequence (e.g. `008_...`).
- The `.down.sql` must fully and safely reverse the `.up.sql`.

## Authoring rules

- Use parameterized application queries at runtime; migrations themselves must never
  embed untrusted or environment-specific literals.
- Prefer explicit column lists and `IF NOT EXISTS` / `IF EXISTS` guards where correct.
- Add indexes for new foreign keys and common query predicates.
- Keep DDL transactional where the operation allows it.
- Do not drop or rename columns/tables without a corresponding reversible `down` step.

## Workflow

- Apply with `make db-migrate-up`; roll back with `make db-migrate-down`;
  check state with `make db-migrate-status`.
- Table/column shapes must stay aligned with `specs/schemas/` and the queries in
  `services/go/data-service` and `airflow/plugins/pociag_processing`.
