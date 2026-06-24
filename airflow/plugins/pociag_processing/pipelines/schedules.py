from __future__ import annotations

from datetime import date
from time import perf_counter
from typing import Any

from pociag_processing.lake import LakeReader
from pociag_processing.models import ProcessResult
from pociag_processing.repository import SyncRepository
from pociag_processing.tracing import get_tracer


def process_schedules(
    date_from: date,
    date_to: date,
    ingestion_run_id: int | None = None,
) -> ProcessResult:
    tracer = get_tracer()
    with tracer.start_as_current_span("schedules.process"):
        repository = SyncRepository()
        lake = LakeReader()

        if repository.is_pipeline_running("schedules", date_from):
            return ProcessResult(
                pipeline="schedules",
                status="failed",
                records_read=0,
                records_written=0,
                records_skipped=0,
                duration_ms=0,
            )

        run_id = repository.create_processing_run("schedules", date_from)
        started = perf_counter()
        try:
            envelopes = lake.read_raw_schedules(date_from, date_to, ingestion_run_id)
            total_read = 0
            total_written = 0
            for env in envelopes:
                payload = env.get("payload", {})
                routes: list[dict[str, Any]] = payload.get("routes", [])
                if not routes:
                    continue
                result = repository.upsert_routes(routes)
                total_read += result.records_read
                total_written += result.records_written

            repository.mark_processing_run_success(run_id, total_read, total_written)
            duration_ms = int((perf_counter() - started) * 1000)
            return ProcessResult(
                pipeline="schedules",
                status="success",
                records_read=total_read,
                records_written=total_written,
                records_skipped=0,
                duration_ms=duration_ms,
            )
        except Exception as exc:
            repository.mark_processing_run_failed(run_id, str(exc))
            raise
