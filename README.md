# Pociag-do-predykcji

Small local usage guide.

## Quick start

1. Set required local environment values in `infra/.env` (PLK base URL and API key).
2. Start local stack:

```bash
make infra-up
```

3. Run database migrations:

```bash
make db-migrate-up
```

4. Show useful local URLs:

```bash
make urls
```

## Common commands

```bash
# Infra
make infra-up
make infra-up-tracing
make infra-up-monitoring
make infra-up-airflow
make infra-up-all
make infra-down

# Database
make db-migrate-up
make db-migrate-down
make db-migrate-status

# Go collector
cd services/go/collector && go test -race -v ./...
cd services/go/collector && golangci-lint run ./...

# Python predictor
cd services/python/predictor && uv run pytest tests/ -v
cd services/python/predictor && uv run ruff check src/ tests/ && uv run mypy src/
```

## OpenCode

Project configuration, specialist agents, workflow commands, and language skills are
versioned in `opencode.json` and `.opencode/`. See [docs/opencode.md](docs/opencode.md)
for installation, authentication, and usage.

## Port mapping

Use this to verify what is reachable from your host.

| Service | Host port | Container port | URL / check | Profile |
| --- | --- | --- | --- | --- |
| Postgres (main) | 5432 | 5432 | localhost:5432 | default |
| Collector (Go) | 8081 | 8081 | http://localhost:8081/metrics | default |
| OTel Collector gRPC | 4317 | 4317 | localhost:4317 | default |
| OTel Collector HTTP | 4318 | 4318 | localhost:4318 | default |
| OTel Collector metrics | 8889 | 8889 | http://localhost:8889/metrics | default |
| Jaeger UI | 16686 | 16686 | http://localhost:16686 | tracing/all |
| Jaeger gRPC | 14250 | 14250 | localhost:14250 | tracing/all |
| Prometheus | 9090 | 9090 | http://localhost:9090 | monitoring/all |
| Grafana | 3000 | 3000 | http://localhost:3000 | monitoring/all |
| Airflow UI | 8090 | 8080 | http://localhost:8090 | airflow/all |
| Postgres (Airflow DB) | 5433 | 5432 | localhost:5433 | airflow/all |

## Airflow login troubleshooting

If you see "Bad Request: The CSRF session token is missing":

1. Ensure AIRFLOW__WEBSERVER__SECRET_KEY is set in infra/.env.
2. Restart Airflow services:

```bash
docker compose -f infra/docker-compose.yml --profile airflow down
docker compose -f infra/docker-compose.yml --profile airflow up -d
```

3. Open Airflow in a private/incognito window or clear cookies for localhost:8090.
