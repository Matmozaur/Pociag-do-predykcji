"""Pure transforms from PLK Open Data payloads (specs/openapi/plk-open-data.json) to table rows.

No I/O here, so every rule about PLK data quirks is unit-testable:

* wall-clock times are Europe/Warsaw; schedule times are ``HH:MM:SS`` plus a day offset;
* planned operation times past midnight can miss their day rollover (see ``align_to_actual``);
* every payload carries a dictionary of the stations it references, so its rows can be written
  with station foreign keys after upserting that dictionary;
* disruption ids are only unique within one response.
"""

from __future__ import annotations

import logging
import re
from collections.abc import Iterable, Mapping
from datetime import date, datetime, timedelta
from typing import Any
from zoneinfo import ZoneInfo

from pociag_processing.models import (
    Disruption,
    DisruptionBatch,
    Operation,
    OperationBatch,
    OperationStop,
    RunKey,
    ScheduleBatch,
    ScheduleStop,
    Station,
    StationCoordinates,
    Train,
    TrainKey,
)

logger = logging.getLogger(__name__)

WARSAW = ZoneInfo("Europe/Warsaw")
TRAIN_STATUSES = frozenset({"S", "P", "C", "X", "Q"})

# .NET TimeSpan as PLK serialises it: [d.]HH:MM[:SS[.fffffff]]
_CLOCK = re.compile(r"^(?:(\d+)\.)?(\d{1,2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?$")


# ── Value parsing ────────────────────────────────────────────────────────────


def parse_clock(value: str | None, day_offset: int | None = None) -> timedelta | None:
    """A schedule time (``HH:MM:SS``) plus its day offset, as an offset from local midnight."""
    if not value:
        return None
    match = _CLOCK.match(value.strip())
    if match is None:
        logger.warning("Unparseable PLK clock time: %r", value)
        return None
    days, hours, minutes, seconds = (int(g) if g else 0 for g in match.groups())
    return timedelta(days=days + (day_offset or 0), hours=hours, minutes=minutes, seconds=seconds)


def parse_local_datetime(value: str | None) -> datetime | None:
    """A PLK timestamp; naive values are Europe/Warsaw wall-clock time."""
    if not value:
        return None
    parsed = datetime.fromisoformat(value)
    if parsed.tzinfo is None:
        # Ambiguous DST fall-back times resolve to the first occurrence (fold=0).
        parsed = parsed.replace(tzinfo=WARSAW)
    return parsed


def align_to_actual(planned: datetime | None, actual: datetime | None) -> datetime | None:
    """Shift ``planned`` by whole days to within 12h of ``actual``.

    PLK sends planned times of stops after midnight on overnight trains with the operating date
    instead of the next day, while actual times and delay minutes are right (GitHub issue #37).
    No real delay or early running comes close to 12 hours.
    """
    if planned is None or actual is None:
        return planned
    days = round((actual - planned) / timedelta(days=1))
    return planned + timedelta(days=days) if days else planned


def _text(value: Any) -> str | None:
    if not isinstance(value, str):
        return None
    stripped = value.strip()
    return stripped or None


def _int(value: Any) -> int | None:
    return value if isinstance(value, int) and not isinstance(value, bool) else None


def _station_names(dictionary: Mapping[str, Any] | None) -> list[Station]:
    """A payload station dictionary: id -> name, or id -> {"id", "name"} in schedules."""
    stations: list[Station] = []
    for key, value in (dictionary or {}).items():
        name = _text(value.get("name") if isinstance(value, Mapping) else value)
        if name is not None:
            stations.append(Station(id=int(key), name=name))
    return stations


# ── Stations ─────────────────────────────────────────────────────────────────


def stations(payload: Mapping[str, Any]) -> list[Station]:
    """``GET /dictionaries/stations`` page."""
    rows: list[Station] = []
    for item in payload.get("stations") or []:
        station_id, name = _int(item.get("id")), _text(item.get("name"))
        if station_id is not None and name is not None:
            rows.append(Station(id=station_id, name=name))
    return rows


def station_cities(payload: Mapping[str, Any]) -> dict[int, str]:
    """``GET /dictionaries/cities``: station id -> city name (PLK sends it upper-case)."""
    cities: dict[int, str] = {}
    for city in payload.get("cities") or []:
        name = _text(city.get("name"))
        if name is None:
            continue
        for station_id in city.get("stationIds") or []:
            cities[int(station_id)] = name.title()
    return cities


def station_coordinates(payload: Mapping[str, Any]) -> list[StationCoordinates]:
    """The packaged ``data/station_coordinates.json``."""
    rows: list[StationCoordinates] = []
    for item in payload.get("stations") or []:
        station_id, lat, lon = item.get("external_id"), item.get("latitude"), item.get("longitude")
        if station_id is None or lat is None or lon is None:
            continue
        rows.append(StationCoordinates(id=int(station_id), latitude=lat, longitude=lon))
    return rows


# ── Schedules ────────────────────────────────────────────────────────────────


def schedules(payload: Mapping[str, Any]) -> ScheduleBatch:
    """``GET /schedules?dateFrom=D&dateTo=D``: trains running on D with their planned stops."""
    dictionaries = payload.get("dictionaries") or {}
    carrier_names = {
        code: name for code, raw in (dictionaries.get("carriers") or {}).items()
        if (name := _text(raw)) is not None
    }
    batch = ScheduleBatch(stations=_station_names(dictionaries.get("stations")))

    trains: dict[TrainKey, Train] = {}
    stops: dict[TrainKey, list[ScheduleStop]] = {}
    for route in payload.get("routes") or []:
        schedule_id, order_id = _int(route.get("scheduleId")), _int(route.get("orderId"))
        if schedule_id is None or order_id is None:
            continue
        key = (schedule_id, order_id)
        carrier_code = _text(route.get("carrierCode"))
        trains[key] = Train(
            schedule_id=schedule_id,
            order_id=order_id,
            name=_text(route.get("name")),
            number=_text(route.get("nationalNumber")),
            category=_text(route.get("commercialCategorySymbol")),
            carrier_code=carrier_code,
            carrier_name=carrier_names.get(carrier_code) if carrier_code else None,
            operating_dates=tuple(
                sorted({date.fromisoformat(d) for d in route.get("operatingDates") or []})
            ),
        )
        # PLK order numbers are sparse (1, 1009, ...); keep their order, renumber from 1.
        route_stops = sorted(route.get("stations") or [], key=lambda s: s.get("orderNumber", 0))
        stops[key] = [
            ScheduleStop(
                train=key,
                seq=seq,
                station_id=stop["stationId"],
                arrival=parse_clock(stop.get("arrivalTime"), stop.get("arrivalDay")),
                departure=parse_clock(stop.get("departureTime"), stop.get("departureDay")),
                platform=_text(stop.get("departurePlatform")) or _text(stop.get("arrivalPlatform")),
                track=_text(stop.get("departureTrack")) or _text(stop.get("arrivalTrack")),
            )
            for seq, stop in enumerate(route_stops, start=1)
        ]

    batch.trains = list(trains.values())
    batch.stops = [stop for route_stops in stops.values() for stop in route_stops]
    return batch


# ── Operations ───────────────────────────────────────────────────────────────


def operations(pages: Iterable[Mapping[str, Any]]) -> OperationBatch:
    """All pages of ``GET /operations?withPlanned=true``. A run seen twice keeps its last copy."""
    station_rows: dict[int, Station] = {}
    runs: dict[RunKey, Operation] = {}
    run_stops: dict[RunKey, dict[int, OperationStop]] = {}

    for page in pages:
        for station in _station_names(page.get("stations")):
            station_rows[station.id] = station
        for train in page.get("trains") or []:
            operation = _operation(train)
            if operation is None:
                continue
            runs[operation.key] = operation
            run_stops[operation.key] = {
                stop.seq: stop
                for raw in train.get("stations") or []
                if (stop := _operation_stop(operation.key, raw)) is not None
            }

    return OperationBatch(
        stations=list(station_rows.values()),
        operations=list(runs.values()),
        stops=[stop for by_seq in run_stops.values() for stop in by_seq.values()],
    )


def _operation(train: Mapping[str, Any]) -> Operation | None:
    schedule_id, order_id = _int(train.get("scheduleId")), _int(train.get("orderId"))
    status, operating_date = train.get("trainStatus"), train.get("operatingDate")
    if schedule_id is None or order_id is None or not operating_date:
        return None
    if status not in TRAIN_STATUSES:
        logger.warning("Skipping operation %s/%s with status %r", schedule_id, order_id, status)
        return None
    return Operation(schedule_id, order_id, date.fromisoformat(operating_date), status)


def _operation_stop(run: RunKey, raw: Mapping[str, Any]) -> OperationStop | None:
    seq, station_id = _int(raw.get("actualSequenceNumber")), _int(raw.get("stationId"))
    if seq is None or station_id is None:
        return None
    actual_arrival = parse_local_datetime(raw.get("actualArrival"))
    actual_departure = parse_local_datetime(raw.get("actualDeparture"))
    return OperationStop(
        run=run,
        seq=seq,
        station_id=station_id,
        planned_arrival=align_to_actual(
            parse_local_datetime(raw.get("plannedArrival")), actual_arrival
        ),
        planned_departure=align_to_actual(
            parse_local_datetime(raw.get("plannedDeparture")), actual_departure
        ),
        actual_arrival=actual_arrival,
        actual_departure=actual_departure,
        arrival_delay=_int(raw.get("arrivalDelayMinutes")),
        departure_delay=_int(raw.get("departureDelayMinutes")),
        is_confirmed=bool(raw.get("isConfirmed", False)),
        is_cancelled=bool(raw.get("isCancelled", False)),
    )


# ── Disruptions ──────────────────────────────────────────────────────────────


def disruptions(payload: Mapping[str, Any]) -> DisruptionBatch:
    """``GET /disruptions``. The affected trains give each disruption its date range.

    Some messages are only a disruption type code (e.g. "utr_55"); they are replaced by that
    type's description from the response's ``disruptionTypes``.
    """
    type_names: Mapping[str, Any] = payload.get("disruptionTypes") or {}
    rows: list[Disruption] = []
    for item in payload.get("disruptions") or []:
        disruption_id, message = _int(item.get("disruptionId")), _text(item.get("message"))
        if disruption_id is None or message is None:
            continue
        type_code = _text(item.get("disruptionTypeCode"))
        if message in type_names:
            type_code = type_code or message
            message = _text(type_names[message]) or message
        affected = {
            (route.get("scheduleId"), route.get("orderId"), route.get("operatingDate"))
            for route in item.get("affectedRoutes") or []
        }
        dates = sorted(date.fromisoformat(d) for _, _, d in affected if d)
        rows.append(
            Disruption(
                id=disruption_id,
                type_code=type_code,
                start_station_id=_int(item.get("startStationId")),
                end_station_id=_int(item.get("endStationId")),
                message=message,
                date_from=dates[0] if dates else None,
                date_to=dates[-1] if dates else None,
                affected_trains=len(affected),
            )
        )
    return DisruptionBatch(stations=_station_names(payload.get("stations")), disruptions=rows)
