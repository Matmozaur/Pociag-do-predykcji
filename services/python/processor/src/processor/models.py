from datetime import date, datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict

PipelineName = Literal["schedules", "operations", "disruptions", "dictionaries"]
ProcessStatus = Literal["success", "partial", "failed"]
RunStatus = Literal["running", "success", "partial", "failed"]


class ErrorResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    error: str
    message: str
    trace_id: str | None = None


class ProcessDictionariesRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    ingestion_run_id: int | None = None


class ProcessSchedulesRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    date_from: date
    date_to: date
    ingestion_run_id: int | None = None


class ProcessOperationsRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    date: date
    ingestion_run_id: int | None = None


class ProcessDisruptionsRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    date_from: date
    date_to: date
    ingestion_run_id: int | None = None


class ProcessingError(BaseModel):
    model_config = ConfigDict(extra="forbid")

    record_ref: str
    reason: str


class ProcessResult(BaseModel):
    model_config = ConfigDict(extra="forbid")

    pipeline: PipelineName
    status: ProcessStatus
    records_read: int
    records_written: int
    records_skipped: int | None = None
    duration_ms: int | None = None
    errors: list[ProcessingError] | None = None


class ProcessingRun(BaseModel):
    model_config = ConfigDict(extra="forbid")

    id: int
    pipeline: PipelineName
    scope_date_from: date | None = None
    scope_date_to: date | None = None
    status: RunStatus
    records_read: int | None = None
    records_written: int | None = None
    records_skipped: int | None = None
    started_at: datetime
    completed_at: datetime | None = None
    error_message: str | None = None


class ProcessingStatusResponse(BaseModel):
    model_config = ConfigDict(extra="forbid")

    runs: list[ProcessingRun] = []
