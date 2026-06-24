from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

PipelineName = Literal["schedules", "operations", "disruptions", "dictionaries"]


@dataclass(slots=True)
class UpsertResult:
    records_read: int
    records_written: int


@dataclass(slots=True)
class ProcessResult:
    pipeline: str
    status: str  # "success", "partial", "failed"
    records_read: int
    records_written: int
    records_skipped: int
    duration_ms: int
