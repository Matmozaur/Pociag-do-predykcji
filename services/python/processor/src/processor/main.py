from __future__ import annotations

import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

import asyncpg
import structlog
import uvicorn
from fastapi import FastAPI
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

from processor.config import Settings
from processor.dependencies import get_settings
from processor.repository import Repository
from processor.router import router

_tracing_configured = False


def configure_logging(level: str) -> None:
    logging.basicConfig(level=getattr(logging, level.upper(), logging.INFO), format="%(message)s")
    structlog.configure(
        processors=[
            structlog.processors.TimeStamper(fmt="iso", key="ts"),
            structlog.processors.add_log_level,
            structlog.processors.JSONRenderer(),
        ],
        logger_factory=structlog.stdlib.LoggerFactory(),
    )


def configure_tracing(settings: Settings) -> None:
    global _tracing_configured
    if _tracing_configured:
        return

    provider = TracerProvider(resource=Resource.create({"service.name": settings.service_name}))
    if settings.otlp_exporter_endpoint:
        exporter = OTLPSpanExporter(endpoint=settings.otlp_exporter_endpoint, insecure=True)
        provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)
    _tracing_configured = True


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    settings = get_settings()
    pool = await asyncpg.create_pool(dsn=settings.database_dsn, min_size=1, max_size=10)
    app.state.pool = pool
    app.state.repository = Repository(pool)
    try:
        yield
    finally:
        await pool.close()


def create_app() -> FastAPI:
    settings = get_settings()
    configure_logging(settings.log_level)
    configure_tracing(settings)

    app = FastAPI(title="Pociag Processor API", version="0.1.0", lifespan=lifespan)
    app.include_router(router)
    FastAPIInstrumentor.instrument_app(app)
    return app


app = create_app()


if __name__ == "__main__":
    uvicorn.run(app, host="0.0.0.0", port=8082)
