# data-service

Domain read API over the curated PostgreSQL tables. Consumed only by the gateway.

Read `services/go/CLAUDE.md` for shared Go rules and commands (`make data-service-test`, `-lint`).

## DB layer — important

- Connection is opened with **GORM** (`gorm.Open(postgres.Open(cfg.DatabaseDSN), …)` in
  `cmd/main.go`), but GORM is used **only as a connection pool**. There are **no GORM models,
  no AutoMigrate, no query builder**.
- Every query is hand-written SQL executed via `r.db.WithContext(ctx).Raw(query, params...).Rows()`
  with `$1, $2, …` positional placeholders. Build dynamic `WHERE` clauses by appending
  `fmt.Sprintf("s.col = $%d", paramIdx)` and growing a `params []any` — never interpolate values.
- `repository.ErrNotFound` is the sentinel; `isNoRows` checks both `sql.ErrNoRows` and
  `gorm.ErrRecordNotFound`. Use `intsToInt32s` for `int[]` → `ANY($n)` params.

## Structure

`internal/repository/` and `internal/handler/` are split per domain
(`dictionaries`, `disruptions`, `operations`, `schedules`), plus a base `repository.go` /
`handler.go`. `internal/model/model.go` holds row structs. Uses the `shared` module
(`dsmodel`, `trainutil`).

## Env vars

Required: `HTTP_ADDR`, `DATABASE_DSN` (`config.Load` errors if unset).
Optional: `OTEL_EXPORTER_OTLP_ENDPOINT`.

## Tests

Unit tests only in this module (no `integration`-tagged tests yet); `make data-service-test`.
Schema shapes must stay aligned with `db/migrations/` and `specs/schemas/`.
