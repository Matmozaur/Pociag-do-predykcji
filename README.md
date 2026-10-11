# Pociąg do Predykcji

Live view of Polish railway traffic built on the [PLK Open Data API](specs/openapi/plk-open-data.json):
a network map with estimated train positions, station boards, timetable search and disruptions.
Historical operations are kept for future delay prediction.

```
PLK Open Data API → Airflow (pociag_processing) → PostgreSQL → api (Go) → frontend (Next.js)
```

See [docs/architecture.md](docs/architecture.md) for how it fits together.

## Quick start

```bash
cp infra/.env.example infra/.env   # set PLK_API_KEY (and passwords)
make up                            # build and start everything
make urls                          # frontend :3100, api :8080, Airflow :8090 (admin/admin)
```

Migrations run on start and the DAGs are unpaused, so data appears within minutes:
operations every 10 min, disruptions every 15 min, stations and timetables daily at 02:30
(trigger `sync_schedules` in Airflow for an immediate first load).

## Commands

Host checks need Docker, Go 1.25+, uv and Node 22. An older `go` (1.21+) still works: with the
default `GOTOOLCHAIN=auto` it downloads go1.25.0 on first use (network needed once). Offline or
with `GOTOOLCHAIN=local`, install Go 1.25 yourself.

```bash
make help        # everything below and more
make test        # all checks CI runs + frontend build
make reset       # stop and DELETE all data volumes
make db-psql     # psql on the curated database
```

SQL tests run when `POCIAG_TEST_DATABASE_URL` points at a disposable database (its `public`
schema is recreated), e.g. `postgres://pociag:pass@localhost:55432/pociag_test`.

## Layout

| Path | What |
|---|---|
| `airflow/` | DAGs + `pociag_processing` plugin (PLK → PostgreSQL) |
| `db/migrations/` | PostgreSQL schema (golang-migrate) |
| `services/go/api/` | Read API for the frontend |
| `services/frontend/` | Next.js UI |
| `specs/` | API contract and pipeline spec |
| `infra/` | docker compose stack, OTel / Prometheus / Grafana config |
