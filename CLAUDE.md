# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

Per-subproject guidance lives in nested `CLAUDE.md` files — read the one for the area you're touching:
`services/go/CLAUDE.md`, `airflow/CLAUDE.md`, `db/migrations/CLAUDE.md`, `services/frontend/CLAUDE.md`.

## What this is

**Pociag do Predykcji** — ingests Polish railway data from the PLK Open Data API with Airflow into
PostgreSQL and serves it to a Next.js frontend through one Go API. Details:
`docs/architecture.md`.

```
PLK API → Airflow (pociag_processing plugin) → PostgreSQL → api (Go) → frontend (Next.js)
```

| Component | Path | Role |
|-----------|------|------|
| pociag_processing | `airflow/plugins/pociag_processing` | Fetches PLK, transforms, upserts curated tables. Runs inside Airflow tasks |
| DAGs | `airflow/dags` | `sync_schedules` (daily), `sync_operations` (10 min), `sync_disruptions` (15 min) |
| schema | `db/migrations` | `stations`, `trains`, `schedule_stops`, `operations`, `operation_stops`, `disruptions` |
| api | `services/go/api` | Read-only JSON API shaped for the frontend |
| frontend | `services/frontend` | Next.js 15; talks only to the api (`/bff/*` proxy) |

Airflow is the only writer; the api only reads. Both depend on the same tables, so a schema
change must keep `repository.py` and `services/go/api/internal/repository` working.

## Contract-first (non-negotiable)

`specs/openapi/api.yml` (api ↔ frontend) and `specs/pipelines.md` (DAGs, PLK data rules) are the
source of truth; `specs/openapi/plk-open-data.json` is the upstream API. Change the spec first,
then the code. On doc conflicts trust: `infra/docker-compose.yml` → `.github/workflows/ci-cd.yaml`
→ `Makefile`. `.github/instructions/*.instructions.md` mirror the nested CLAUDE.md files.

## Scope discipline — keep the diff small

The change you ship should be the smallest one that does the task. Fewer touched files = faster
review, easier revert, cleaner history.

- **Out-of-scope findings → issue, not inline fix.** If you spot a bug or needed improvement
  unrelated to the current task, do **not** fix it inline. Run `gh issue create` describing the
  finding, its location (`file:line`), and why it matters, then continue what you were doing.
  Mention the issue number in your summary.
- **No drive-by edits.** Don't reformat, rename, re-order imports, "tidy" comments, bump
  dependencies, or restructure code you're only reading. If a formatter/linter rewrites files you
  didn't touch, revert those hunks before committing.
- **Match the surrounding code.** Follow the existing patterns, naming, and structure of the file
  you're in rather than introducing a new style — even one you'd prefer.
- **Don't widen the interface.** No new endpoints / fields / events / columns / config keys / CLI
  flags / exported functions beyond what the task needs (and the specs allow — see Contract-first).
- **Ask before large refactors.** If the task seems to require touching many files or moving code
  across modules, stop and confirm the approach with the user first.
- **Leave TODOs and dead code alone** unless removing them *is* the task. File an issue instead.
- **Tests and docs for what you changed**, not a sweep of pre-existing gaps.

## Repo-wide workflow

```bash
cp infra/.env.example infra/.env    # PLK_API_KEY, POSTGRES_PASSWORD, Airflow secrets
make up                              # whole stack; migrations run, DAGs start unpaused
make test                            # api + airflow checks, frontend build
make help                            # all targets
```

SQL tests (Go and Python) run only when `POCIAG_TEST_DATABASE_URL` points at a disposable
database — they recreate its `public` schema. Never point it at the stack's database.

### Runtime ports (host)

| | Host | | Host |
|---|---|---|---|
| Postgres (main) | 5434 | Airflow Postgres | 5433 |
| api | 8080 | Airflow UI | 8090 |
| frontend | 3100 | Jaeger / Prometheus / Grafana | 16686 / 9090 / 3001 |

### Branch & commit conventions (observed)

- Branches: `feature/<slug>`, `fix/<slug>`, `chore/<slug>` (kebab-case). Some older ones are bare
  (`gateway_v1`, `basic-fe`) — prefer the prefixed form.
- Commits: short capitalized imperative subject, no Conventional-Commits prefix
  (`Add stations on the map`, `Fix ...`, `Update ...`, `Remove ...`). Keep them scoped.
- Merge to `main` via GitHub PR ("Merge pull request #N from Matmozaur/<branch>").
- CI (`.github/workflows/ci-cd.yaml`) runs on every branch push: Go test + lint for the api and
  ruff / mypy / pytest for `airflow/`, both with a Postgres service for the SQL tests. Build &
  deploy stages are placeholders.

## Cross-cutting conventions

- **No hardcoded secrets** — env vars (Go: `os.LookupEnv`) or Airflow connections.
- **Tracing**: service names `pociag.api` / `pociag.airflow`, span names `<noun>.<verb>`, OTLP gRPC
  to `OTEL_EXPORTER_OTLP_ENDPOINT`.
- **Structured JSON logs**: `zap` in Go, `logging` in Python. Never log secrets or payloads.
- **Parameterized SQL only**, on both sides.

## Shell

Work inside WSL at `/mnt/c/Users/Admin/Documents/IT/Pociag-do-predykcji`; run commands directly,
not via `wsl bash -c "..."` wrappers.
