# db/migrations

PostgreSQL schema, managed with **golang-migrate**. This is the single schema shared by
`data-service` (reads) and `airflow/plugins/pociag_processing` (writes).

## Rules

- Every migration is a pair: `NNN_description.up.sql` + `NNN_description.down.sql`, `NNN`
  zero-padded and monotonically increasing (next is `010_`). `.down.sql` must fully and safely
  reverse `.up.sql`.
- Use `IF [NOT] EXISTS` guards and explicit column lists. Add indexes for new FKs and for
  predicates the queries actually filter on (check `data-service/internal/repository/*.go` and
  `pociag_processing/repository.py`).
- Keep DDL transactional where the operation allows.
- Don't drop/rename a column or table without a reversible `down` step.
- Table/column shapes must stay aligned with `specs/schemas/*.json`.

## Running (from repo root; Postgres must be up: `make infra-up`)

```bash
make db-migrate-up        # apply all pending
make db-migrate-down      # roll back exactly ONE
make db-migrate-status    # current version
make db-psql              # psql shell
```

These run the `migrate/migrate:v4.18.1` Docker image with `--network host` against `DB_URL`,
default `postgres://pociag:pociag_dev_secret@127.0.0.1:5434/pociag?sslmode=disable`
(host port **5434**). Override with `make db-migrate-up DB_URL=...`.

There is no separate migrations runner in CI — apply locally and verify both consumers still
build/test before opening a PR.
