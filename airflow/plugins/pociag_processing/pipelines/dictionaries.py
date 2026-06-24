from __future__ import annotations

from datetime import date
from time import perf_counter
from typing import Any

from pociag_processing.lake import LakeReader
from pociag_processing.models import ProcessResult, UpsertResult
from pociag_processing.repository import SyncRepository
from pociag_processing.tracing import get_tracer


def _upsert_dictionary(
    repository: SyncRepository,
    dtype: str,
    payload: dict[str, Any],
) -> UpsertResult:
    dict_key_map: dict[str, str] = {
        "carriers": "carriers",
        "stations": "stations",
        "commercial_categories": "commercialCategories",
        "stop_types": "stopTypes",
    }
    payload_key = dict_key_map.get(dtype)
    if payload_key is None:
        return UpsertResult(records_read=0, records_written=0)

    records: list[dict[str, Any]] = payload.get(payload_key, [])
    if not records:
        return UpsertResult(records_read=0, records_written=0)

    if dtype == "carriers":
        return repository.upsert_carriers(records)
    if dtype == "stations":
        return repository.upsert_stations(records)
    if dtype == "commercial_categories":
        return repository.upsert_commercial_categories(records)
    if dtype == "stop_types":
        return repository.upsert_stop_types(records)
    return UpsertResult(records_read=0, records_written=0)


def process_dictionaries(run_date: date, ingestion_run_id: int | None = None) -> ProcessResult:
    tracer = get_tracer()
    with tracer.start_as_current_span("dictionaries.process"):
        repository = SyncRepository()
        lake = LakeReader()

        if repository.is_pipeline_running("dictionaries", run_date):
            return ProcessResult(
                pipeline="dictionaries",
                status="failed",
                records_read=0,
                records_written=0,
                records_skipped=0,
                duration_ms=0,
            )

        run_id = repository.create_processing_run("dictionaries", run_date)
        started = perf_counter()
        try:
            envelopes = lake.read_raw_dictionaries(ingestion_run_id, run_date)
            total_read = 0
            total_written = 0
            for env in envelopes:
                meta = env.get("metadata", {})
                dtype = meta.get("dictionary_type", "")
                payload = env.get("payload", {})
                result = _upsert_dictionary(repository, dtype, payload)
                total_read += result.records_read
                total_written += result.records_written

            repository.mark_processing_run_success(run_id, total_read, total_written)
            duration_ms = int((perf_counter() - started) * 1000)
            return ProcessResult(
                pipeline="dictionaries",
                status="success",
                records_read=total_read,
                records_written=total_written,
                records_skipped=0,
                duration_ms=duration_ms,
            )
        except Exception as exc:
            repository.mark_processing_run_failed(run_id, str(exc))
            raise
