FROM python:3.13-slim AS builder

ENV UV_LINK_MODE=copy

RUN apt-get update && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN curl -LsSf https://astral.sh/uv/install.sh | sh
ENV PATH="/root/.local/bin:${PATH}"

WORKDIR /app
COPY pyproject.toml ./
RUN uv export --format requirements-txt --no-hashes --output-file requirements.txt
RUN uv pip install --system -r requirements.txt

FROM python:3.13-slim

RUN addgroup --system appgroup && adduser --system --uid 1001 --ingroup appgroup appuser

WORKDIR /app
COPY --from=builder /usr/local/lib/python3.13 /usr/local/lib/python3.13
COPY --from=builder /usr/local/bin /usr/local/bin
COPY src ./src

ENV PYTHONPATH=/app/src
EXPOSE 8082
USER appuser

CMD ["python", "-m", "processor.main"]
