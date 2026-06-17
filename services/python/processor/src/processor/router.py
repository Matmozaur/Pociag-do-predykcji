from __future__ import annotations

import asyncio
from collections.abc import Awaitable, Callable
from datetime import UTC, date, datetime
from functools import partial
from time import perf_counter
from typing import Annotated, Any, cast

from fastapi import APIRouter, Depends, Query
from fastapi.responses import JSONResponse, Response
from opentelemetry import trace
from opentelemetry.trace import Status, StatusCode, Tracer

from processor.dependencies import get_lake_reader, get_repository, get_tracer
from processor.lake import LakeReader
from processor.models import (
    ErrorResponse,
    PipelineName,
    ProcessDictionariesRequest,
    ProcessDisruptionsRequest,
    ProcessingRun,
    ProcessingStatusResponse,
    ProcessOperationsRequest,
    ProcessResult,
    ProcessSchedulesRequest,
    RunStatus,
)
from processor.repository import Repository, UpsertResult

router = APIRouter()


def _current_trace_id() -> str | None:
    span = trace.get_current_span()
    span_context = span.get_span_context()
    if not span_context.is_valid:
        return None
    return f"{span_context.trace_id:032x}"


def _error_response(status_code: int, error: str, message: str) -> JSONResponse:
    payload = ErrorResponse(error=error, message=message, trace_id=_current_trace_id())
    return JSONResponse(status_code=status_code, content=payload.model_dump(exclude_none=True))


_T = Any


async def _sync_to_async(fn: Callable[..., _T], *args: Any) -> _T:
    loop = asyncio.get_running_loop()
    return cast(_T, await loop.run_in_executor(None, partial(fn, *args)))


def _coerce_run_status(value: str) -> RunStatus:
    if value in {"running", "success", "partial", "failed"}:
        return cast(RunStatus, value)
    return "failed"


async def _run_pipeline(
    *,
    tracer: Tracer,
    repository: Repository,
    pipeline: PipelineName,
    run_date: date,
    process_fn: Callable[[], Awaitable[UpsertResult]],
) -> ProcessResult | JSONResponse:
    with tracer.start_as_current_span(f"{pipeline}.process") as span:
        if await repository.is_pipeline_running(pipeline, run_date):
            span.set_attribute("processor.conflict", True)
            return _error_response(409, "already_running", "processing already running")

        run_id = await repository.create_processing_run(pipeline, run_date)
        started = perf_counter()
        try:
            result = await process_fn()
            await repository.mark_processing_run_success(
                run_id, result.records_read, result.records_written,
            )
            duration_ms = int((perf_counter() - started) * 1000)
            return ProcessResult(
                pipeline=pipeline,
                status="success",
                records_read=result.records_read,
                records_written=result.records_written,
                records_skipped=0,
                duration_ms=duration_ms,
            )
        except Exception as exc:
            span.record_exception(exc)
            span.set_status(Status(StatusCode.ERROR, str(exc)))
            await repository.mark_processing_run_failed(run_id, str(exc))
            return _error_response(500, "processing_error", "processing error")


@router.get("/healthz")
async def healthz() -> Response:
    return Response(status_code=200)


@router.get("/readyz")
async def readyz(
    repository: Annotated[Repository, Depends(get_repository)],
) -> Response:
    try:
        await repository.ping()
    except Exception:
        return _error_response(503, "not_ready", "service is not ready")
    return Response(status_code=200)


@router.post(
    "/api/v1/process/dictionaries",
    response_model=ProcessResult,
    responses={409: {"model": ErrorResponse}, 500: {"model": ErrorResponse}},
)
async def process_dictionaries(
    request: ProcessDictionariesRequest | None,
    repository: Annotated[Repository, Depends(get_repository)],
    lake_reader: Annotated[LakeReader, Depends(get_lake_reader)],
    tracer: Annotated[Tracer, Depends(get_tracer)],
) -> ProcessResult | JSONResponse:
    ingestion_run_id = request.ingestion_run_id if request is not None else None
    run_date = datetime.now(tz=UTC).date()

    async def _process() -> UpsertResult:
        envelopes = await _sync_to_async(
            lake_reader.read_raw_dictionaries, ingestion_run_id, run_date,
        )
        total_read = 0
        total_written = 0
        for env in envelopes:
            meta = env.get("metadata", {})
            dtype = meta.get("dictionary_type", "")
            payload = env.get("payload", {})
            result = await _upsert_dictionary(repository, dtype, payload)
            total_read += result.records_read
            total_written += result.records_written
        return UpsertResult(
            records_read=total_read, records_written=total_written,
        )

    return await _run_pipeline(
        tracer=tracer,
        repository=repository,
        pipeline="dictionaries",
        run_date=run_date,
        process_fn=_process,
    )


async def _upsert_dictionary(
    repository: Repository,
    dtype: str,
    payload: dict[str, Any],
) -> UpsertResult:
    """Dispatch to the correct upsert based on dictionary type."""
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
        return await repository.upsert_carriers(records)
    if dtype == "stations":
        return await repository.upsert_stations(records)
    if dtype == "commercial_categories":
        return await repository.upsert_commercial_categories(records)
    if dtype == "stop_types":
        return await repository.upsert_stop_types(records)
    return UpsertResult(records_read=0, records_written=0)


@router.post(
    "/api/v1/process/schedules",
    response_model=ProcessResult,
    responses={
        400: {"model": ErrorResponse},
        409: {"model": ErrorResponse},
        500: {"model": ErrorResponse},
    },
)
async def process_schedules(
    request: ProcessSchedulesRequest,
    repository: Annotated[Repository, Depends(get_repository)],
    lake_reader: Annotated[LakeReader, Depends(get_lake_reader)],
    tracer: Annotated[Tracer, Depends(get_tracer)],
) -> ProcessResult | JSONResponse:
    if request.date_from > request.date_to:
        return _error_response(
            400,
            "invalid_request",
            "date_from must be before or equal to date_to",
        )

    async def _process() -> UpsertResult:
        envelopes = await _sync_to_async(
            lake_reader.read_raw_schedules,
            request.date_from,
            request.date_to,
            request.ingestion_run_id,
        )
        total_read = 0
        total_written = 0
        for env in envelopes:
            payload = env.get("payload", {})
            routes: list[dict[str, Any]] = payload.get("routes", [])
            if not routes:
                continue
            result = await repository.upsert_routes(routes)
            total_read += result.records_read
            total_written += result.records_written
        return UpsertResult(
            records_read=total_read, records_written=total_written,
        )

    return await _run_pipeline(
        tracer=tracer,
        repository=repository,
        pipeline="schedules",
        run_date=request.date_from,
        process_fn=_process,
    )


@router.post(
    "/api/v1/process/operations",
    response_model=ProcessResult,
    responses={
        400: {"model": ErrorResponse},
        409: {"model": ErrorResponse},
        500: {"model": ErrorResponse},
    },
)
async def process_operations(
    request: ProcessOperationsRequest,
    repository: Annotated[Repository, Depends(get_repository)],
    lake_reader: Annotated[LakeReader, Depends(get_lake_reader)],
    tracer: Annotated[Tracer, Depends(get_tracer)],
) -> ProcessResult | JSONResponse:
    async def _process() -> UpsertResult:
        envelopes = await _sync_to_async(
            lake_reader.read_raw_operations,
            request.date,
            request.ingestion_run_id,
        )
        total_read = sum(
            int(e.get("metadata", {}).get("record_count", 0))
            for e in envelopes
        )
        return UpsertResult(records_read=total_read, records_written=0)

    return await _run_pipeline(
        tracer=tracer,
        repository=repository,
        pipeline="operations",
        run_date=request.date,
        process_fn=_process,
    )


@router.post(
    "/api/v1/process/disruptions",
    response_model=ProcessResult,
    responses={
        400: {"model": ErrorResponse},
        409: {"model": ErrorResponse},
        500: {"model": ErrorResponse},
    },
)
async def process_disruptions(
    request: ProcessDisruptionsRequest,
    repository: Annotated[Repository, Depends(get_repository)],
    lake_reader: Annotated[LakeReader, Depends(get_lake_reader)],
    tracer: Annotated[Tracer, Depends(get_tracer)],
) -> ProcessResult | JSONResponse:
    if request.date_from > request.date_to:
        return _error_response(
            400,
            "invalid_request",
            "date_from must be before or equal to date_to",
        )

    async def _process() -> UpsertResult:
        envelopes = await _sync_to_async(
            lake_reader.read_raw_disruptions,
            request.date_from,
            request.date_to,
            request.ingestion_run_id,
        )
        total_read = sum(
            int(e.get("metadata", {}).get("record_count", 0))
            for e in envelopes
        )
        return UpsertResult(records_read=total_read, records_written=0)

    return await _run_pipeline(
        tracer=tracer,
        repository=repository,
        pipeline="disruptions",
        run_date=request.date_from,
        process_fn=_process,
    )


@router.get("/api/v1/process/status", response_model=ProcessingStatusResponse)
async def get_processing_status(
    repository: Annotated[Repository, Depends(get_repository)],
    tracer: Annotated[Tracer, Depends(get_tracer)],
    pipeline: Annotated[PipelineName | None, Query()] = None,
    limit: Annotated[int, Query(ge=1, le=100)] = 10,
) -> ProcessingStatusResponse:
    with tracer.start_as_current_span("process.status.list"):
        rows = await repository.list_processing_runs(pipeline, limit)
        runs = [
            ProcessingRun(
                id=row.id,
                pipeline=row.pipeline,
                scope_date_from=row.run_date,
                scope_date_to=row.run_date,
                status=_coerce_run_status(row.status),
                records_read=row.records_fetched,
                records_written=row.records_upserted,
                records_skipped=0,
                started_at=row.started_at,
                completed_at=row.completed_at,
                error_message=row.error_message,
            )
            for row in rows
        ]
        return ProcessingStatusResponse(runs=runs)
