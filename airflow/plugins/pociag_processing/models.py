"""Rows written to the curated tables (db/migrations/001_init.up.sql), one dataclass per table."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import date, datetime, timedelta

TrainKey = tuple[int, int]
"""PLK route identity: (schedule_id, order_id)."""

RunKey = tuple[int, int, date]
"""A train's run: (schedule_id, order_id, operating_date)."""


@dataclass(frozen=True, slots=True)
class Station:
    id: int
    name: str


@dataclass(frozen=True, slots=True)
class StationCoordinates:
    id: int
    latitude: float
    longitude: float


@dataclass(frozen=True, slots=True)
class Train:
    schedule_id: int
    order_id: int
    name: str | None
    number: str | None
    category: str | None
    carrier_code: str | None
    carrier_name: str | None
    operating_dates: tuple[date, ...]

    @property
    def key(self) -> TrainKey:
        return (self.schedule_id, self.order_id)


@dataclass(frozen=True, slots=True)
class ScheduleStop:
    train: TrainKey
    seq: int
    station_id: int
    arrival: timedelta | None
    departure: timedelta | None
    platform: str | None
    track: str | None


@dataclass(frozen=True, slots=True)
class Operation:
    schedule_id: int
    order_id: int
    operating_date: date
    status: str

    @property
    def key(self) -> RunKey:
        return (self.schedule_id, self.order_id, self.operating_date)


@dataclass(frozen=True, slots=True)
class OperationStop:
    run: RunKey
    seq: int
    station_id: int
    planned_arrival: datetime | None
    planned_departure: datetime | None
    actual_arrival: datetime | None
    actual_departure: datetime | None
    arrival_delay: int | None
    departure_delay: int | None
    is_confirmed: bool
    is_cancelled: bool


@dataclass(frozen=True, slots=True)
class Disruption:
    id: int
    type_code: str | None
    start_station_id: int | None
    end_station_id: int | None
    message: str
    date_from: date | None
    date_to: date | None
    affected_trains: int


@dataclass(slots=True)
class ScheduleBatch:
    stations: list[Station] = field(default_factory=list)
    trains: list[Train] = field(default_factory=list)
    stops: list[ScheduleStop] = field(default_factory=list)


@dataclass(slots=True)
class OperationBatch:
    stations: list[Station] = field(default_factory=list)
    operations: list[Operation] = field(default_factory=list)
    stops: list[OperationStop] = field(default_factory=list)


@dataclass(slots=True)
class DisruptionBatch:
    stations: list[Station] = field(default_factory=list)
    disruptions: list[Disruption] = field(default_factory=list)
