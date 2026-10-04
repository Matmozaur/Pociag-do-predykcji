---
description: "Database migration conventions (golang-migrate)."
applyTo: "db/migrations/**/*.sql"
---

<!-- Mirror of db/migrations/CLAUDE.md -->

# db/migrations

PostgreSQL schema (golang-migrate), written by `airflow/plugins/pociag_processing/repository.py`
and read by `services/go/api/internal/repository`. Overview: `docs/architecture.md`.

## Rules

- Pairs `NNN_description.up.sql` / `.down.sql`, zero-padded and increasing (next: `002_`).
  `.down.sql` fully reverses `.up.sql`; keep DDL in a transaction.
- Index new foreign keys and the predicates the two repositories filter on.
- After a change, run both SQL test suites (`POCIAG_TEST_DATABASE_URL=… make api-test
  airflow-test`); they apply `001_init.up.sql` and later migrations must keep them passing.

## Running (from repo root)

`docker compose` applies migrations on start (`migrate` service). Manually:
`make db-migrate-up | db-migrate-down | db-migrate-status | db-psql` against `DB_URL`
(default `postgres://pociag:pociag_dev_secret@127.0.0.1:5434/pociag?sslmode=disable`).
