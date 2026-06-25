from __future__ import annotations

from datetime import date
from time import perf_counter
from typing import Any

from pociag_processing.lake import LakeReader
from pociag_processing.models import ProcessResult
from pociag_processing.repository import SyncRepository
from pociag_processing.tracing import get_tracer


def process_operations(
    operating_date: date,
    ingestion_run_id: int | None = None,
) -> ProcessResult:
    tracer = get_tracer()
    with tracer.start_as_current_span("operations.process"):
        repository = SyncRepository()
        lake = LakeReader()

        if repository.is_pipeline_running("operations", operating_date):
            return ProcessResult(
                pipeline="operations",
                status="failed",
                records_read=0,
                records_written=0,
                records_skipped=0,
                duration_ms=0,
            )

        run_id = repository.create_processing_run("operations", operating_date)
        started = perf_counter()
        try:
            envelopes = lake.read_raw_operations(operating_date, ingestion_run_id)
            total_read = 0
            total_written = 0
            for env in envelopes:
                payload = env.get("payload", {})
                operations: list[dict[str, Any]] = payload.get("operations", [])
                if not operations:
                    continue
                result = repository.upsert_operations(operations, operating_date)
                total_read += result.records_read
                total_written += result.records_written

            repository.mark_processing_run_success(run_id, total_read, total_written)
            duration_ms = int((perf_counter() - started) * 1000)
            return ProcessResult(
                pipeline="operations",
                status="success",
                records_read=total_read,
                records_written=total_written,
                records_skipped=0,
                duration_ms=duration_ms,
            )
        except Exception as exc:
            repository.mark_processing_run_failed(run_id, str(exc))
            raise
