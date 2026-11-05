from __future__ import annotations

import os
from collections.abc import Iterator
from contextlib import contextmanager

from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.sdk.resources import Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor

FLUSH_TIMEOUT_MILLIS = 5000  # an unreachable collector must not hold a task up for long

_configured = False


def _configure_once() -> None:
    global _configured  # noqa: PLW0603
    if _configured:
        return
    _configured = True

    endpoint = os.environ.get("OTEL_EXPORTER_OTLP_ENDPOINT")
    if not endpoint:
        return

    resource = Resource.create({"service.name": "pociag.airflow"})
    provider = TracerProvider(resource=resource)
    exporter = OTLPSpanExporter(endpoint=endpoint, insecure=True)
    provider.add_span_processor(BatchSpanProcessor(exporter))
    trace.set_tracer_provider(provider)


def get_tracer() -> trace.Tracer:
    _configure_once()
    return trace.get_tracer("pociag.airflow")


@contextmanager
def sync_span(name: str) -> Iterator[None]:
    """A sync's root span, exported before the task process exits, on failure too.

    LocalExecutor's forked task process ends with ``os._exit()``, which skips the provider's atexit
    shutdown and so drops whatever the BatchSpanProcessor still holds, always including this span.
    """
    try:
        with get_tracer().start_as_current_span(name):
            yield
    finally:
        provider = trace.get_tracer_provider()
        if isinstance(provider, TracerProvider):
            provider.force_flush(FLUSH_TIMEOUT_MILLIS)
