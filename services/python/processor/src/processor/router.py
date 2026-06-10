from __future__ import annotations

from collections.abc import Awaitable, Callable
from datetime import UTC, date, datetime
from time import perf_counter
from typing import Annotated, cast

from fastapi import APIRouter, Depends, Query
from fastapi.responses import JSONResponse, Response
from opentelemetry import trace
from opentelemetry.trace import Status, StatusCode, Tracer

from processor.dependencies import get_repository, get_tracer
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
from processor.repository import Repository

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
    count_fn: Callable[[], Awaitable[int]],
) -> ProcessResult | JSONResponse:
    with tracer.start_as_current_span(f"{pipeline}.process") as span:
        if await repository.is_pipeline_running(pipeline, run_date):
            span.set_attribute("processor.conflict", True)
            return _error_response(409, "already_running", "processing already running")

        run_id = await repository.create_processing_run(pipeline, run_date)
        started = perf_counter()
        try:
            records_read = int(await count_fn())
            records_written = records_read
            await repository.mark_processing_run_success(run_id, records_read, records_written)
            duration_ms = int((perf_counter() - started) * 1000)
            return ProcessResult(
                pipeline=pipeline,
                status="success",
                records_read=records_read,
                records_written=records_written,
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
    tracer: Annotated[Tracer, Depends(get_tracer)],
) -> ProcessResult | JSONResponse:
    ingestion_run_id = request.ingestion_run_id if request is not None else None
    return await _run_pipeline(
        tracer=tracer,
        repository=repository,
        pipeline="dictionaries",
        run_date=datetime.now(tz=UTC).date(),
        count_fn=lambda: repository.count_raw_dictionaries(ingestion_run_id),
    )


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
    tracer: Annotated[Tracer, Depends(get_tracer)],
) -> ProcessResult | JSONResponse:
    if request.date_from > request.date_to:
        return _error_response(
            400,
            "invalid_request",
            "date_from must be before or equal to date_to",
        )

    return await _run_pipeline(
        tracer=tracer,
        repository=repository,
        pipeline="schedules",
        run_date=request.date_from,
        count_fn=lambda: repository.count_raw_schedules(
            request.date_from,
            request.date_to,
            request.ingestion_run_id,
        ),
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
    tracer: Annotated[Tracer, Depends(get_tracer)],
) -> ProcessResult | JSONResponse:
    return await _run_pipeline(
        tracer=tracer,
        repository=repository,
        pipeline="operations",
        run_date=request.date,
        count_fn=lambda: repository.count_raw_operations(
            request.date,
            request.ingestion_run_id,
        ),
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
    tracer: Annotated[Tracer, Depends(get_tracer)],
) -> ProcessResult | JSONResponse:
    if request.date_from > request.date_to:
        return _error_response(
            400,
            "invalid_request",
            "date_from must be before or equal to date_to",
        )

    return await _run_pipeline(
        tracer=tracer,
        repository=repository,
        pipeline="disruptions",
        run_date=request.date_from,
        count_fn=lambda: repository.count_raw_disruptions(
            request.date_from,
            request.date_to,
            request.ingestion_run_id,
        ),
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
