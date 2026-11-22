# services/python/predictor — delay predictor

Internal FastAPI service called only by the Go api (`PREDICTOR_URL`). Contract:
`specs/openapi/predictor.yml`; how the api uses it: `docs/architecture.md` (Serving).

For a train (route) and a stop it returns the probability of each of the seven arrival delay
ranges: the share of the route's confirmed, not cancelled arrivals at that stop over the last 7
operating dates (today included, Europe/Warsaw) that fell in each. No such arrival → a fixed
placeholder with `is_mock: true`. It only reads the curated tables; its DB session is read-only.

## Layout (`src/pociag_predictor/`)

- `model.py` — pure: `RANGES`, `range_index`, `from_history`, the placeholder.
- `predictor.py` — the 7-day window and the Warsaw operating date; `ArrivalHistory` protocol.
- `repository.py` — all SQL (psycopg 3 pool, parameterised, `default_transaction_read_only`).
- `app.py` — `create_app(store, clock)`: `/healthz`, `/readyz` (DB ping), `POST /v1/predict-delay`.
- `main.py` — `build_app()` for `uvicorn --factory`; `tracing.py` — OTLP as `pociag.predictor`.
- `tests/` — `test_model.py`, `test_app.py` (fake store), `test_repository_integration.py` (needs
  `POCIAG_TEST_DATABASE_URL`; recreates that DB's `public` schema).

## Commands (from repo root)

```bash
make predictor-test    # uv sync + pytest (set POCIAG_TEST_DATABASE_URL to include the SQL tests)
make predictor-lint    # ruff + mypy --strict
```

## Conventions

- `from __future__ import annotations`, full annotations, `mypy --strict` clean.
- Env: `DATABASE_DSN` (required, libpq URL), `OTEL_EXPORTER_OTLP_ENDPOINT` (optional).
- Keep the SQL aligned with `db/migrations/`. Never log payloads or the DSN.
- Changing the response means changing `specs/openapi/predictor.yml` and the Go client
  (`services/go/api/internal/prediction/client.go`, which validates it) first.
