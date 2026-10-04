"""Sync entrypoints called by the DAG tasks: fetch from PLK, transform, write to PostgreSQL.

Each returns a small summary for the task log / XCom. ``client`` and ``repository`` default to
the real ones and are injectable for tests.
"""

from __future__ import annotations

import json
import logging
from collections.abc import Sequence
from datetime import date, timedelta
from importlib import resources
from typing import Any

from pociag_processing import transform
from pociag_processing.models import Station
from pociag_processing.plk import PlkClient
from pociag_processing.repository import Repository
from pociag_processing.tracing import get_tracer

logger = logging.getLogger(__name__)


def load_station_coordinates() -> dict[str, Any]:
    resource = resources.files("pociag_processing").joinpath("data/station_coordinates.json")
    payload: dict[str, Any] = json.loads(resource.read_text(encoding="utf-8"))
    return payload


def sync_stations(
    client: PlkClient | None = None, repository: Repository | None = None
) -> dict[str, int]:
    """PLK station dictionary + cities + curated coordinates."""
    client, repository = client or PlkClient(), repository or Repository()
    with get_tracer().start_as_current_span("stations.sync"):
        stations: list[Station] = []
        for page in client.stations():
            stations.extend(transform.stations(page))
        cities = transform.station_cities(client.cities())
        coordinates = transform.station_coordinates(load_station_coordinates())
        repository.save_stations(stations, cities, coordinates)
    summary = {"stations": len(stations), "cities": len(cities), "coordinates": len(coordinates)}
    logger.info("Stations synced: %s", summary)
    return summary


def sync_schedules(
    days: Sequence[date], client: PlkClient | None = None, repository: Repository | None = None
) -> dict[str, int]:
    """Trains and planned stops for each day, one PLK request and one transaction per day."""
    client, repository = client or PlkClient(), repository or Repository()
    summary = {"days": 0, "trains": 0, "stops": 0}
    with get_tracer().start_as_current_span("schedules.sync"):
        for day in days:
            batch = transform.schedules(client.schedules(day))
            repository.save_schedules(batch)
            summary["days"] += 1
            summary["trains"] += len(batch.trains)
            summary["stops"] += len(batch.stops)
            logger.info("Schedules for %s: %d trains", day, len(batch.trains))
    return summary


def sync_operations(
    client: PlkClient | None = None, repository: Repository | None = None
) -> dict[str, int]:
    """The current PLK operations snapshot."""
    client, repository = client or PlkClient(), repository or Repository()
    with get_tracer().start_as_current_span("operations.sync"):
        batch = transform.operations(client.operations())
        repository.save_operations(batch)
    summary = {"operations": len(batch.operations), "stops": len(batch.stops)}
    logger.info("Operations synced: %s", summary)
    return summary


def sync_disruptions(
    today: date, client: PlkClient | None = None, repository: Repository | None = None
) -> dict[str, int]:
    """Replace the disruption snapshot with the disruptions affecting today and tomorrow."""
    client, repository = client or PlkClient(), repository or Repository()
    with get_tracer().start_as_current_span("disruptions.sync"):
        batch = transform.disruptions(client.disruptions(today, today + timedelta(days=1)))
        repository.replace_disruptions(batch)
    summary = {"disruptions": len(batch.disruptions)}
    logger.info("Disruptions synced: %s", summary)
    return summary
