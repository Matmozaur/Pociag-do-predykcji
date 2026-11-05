from __future__ import annotations

import json
from collections.abc import Iterator
from datetime import date
from pathlib import Path
from typing import Any, cast
from unittest.mock import patch

import pytest
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.sdk.trace.export.in_memory_span_exporter import InMemorySpanExporter
from pociag_processing import pipelines
from pociag_processing.plk import PlkClient
from pociag_processing.repository import Repository

FIXTURES = Path(__file__).parent / "fixtures"


def load(name: str) -> Any:
    return json.loads((FIXTURES / name).read_text(encoding="utf-8"))


class FakePlk:
    def __init__(self) -> None:
        self.schedule_days: list[date] = []
        self.disruption_range: tuple[date, date] | None = None

    def stations(self) -> Iterator[dict[str, Any]]:
        yield load("stations.json")

    def cities(self) -> dict[str, Any]:
        return cast(dict[str, Any], load("cities.json"))

    def schedules(self, day: date) -> dict[str, Any]:
        self.schedule_days.append(day)
        return cast(dict[str, Any], load("schedules.json"))

    def operations(self) -> Iterator[dict[str, Any]]:
        yield load("operations_page1.json")
        yield load("operations_page2.json")

    def disruptions(self, date_from: date, date_to: date) -> dict[str, Any]:
        self.disruption_range = (date_from, date_to)
        return cast(dict[str, Any], load("disruptions.json"))


class RecordingRepository:
    def __init__(self) -> None:
        self.calls: list[tuple[str, Any]] = []

    def __getattr__(self, name: str) -> Any:
        return lambda *args: self.calls.append((name, args))


class BrokenRepository:
    def save_operations(self, batch: Any) -> None:
        raise RuntimeError("database down")


def run(fn: Any, *args: Any) -> tuple[dict[str, int], FakePlk, RecordingRepository]:
    plk, repo = FakePlk(), RecordingRepository()
    summary = fn(*args, client=cast(PlkClient, plk), repository=cast(Repository, repo))
    return summary, plk, repo


def test_sync_stations() -> None:
    summary, _, repo = run(pipelines.sync_stations)

    assert summary["stations"] == 3
    assert summary["cities"] == 12
    assert summary["coordinates"] > 1000
    [(name, (stations, cities, coordinates))] = repo.calls
    assert name == "save_stations"
    assert len(stations) == 3 and len(cities) == 12 and len(coordinates) == summary["coordinates"]


def test_sync_schedules_writes_one_batch_per_day() -> None:
    days = [date(2026, 10, 4), date(2026, 10, 5)]
    summary, plk, repo = run(pipelines.sync_schedules, days)

    assert plk.schedule_days == days
    assert [name for name, _ in repo.calls] == ["save_schedules", "save_schedules"]
    assert summary == {"days": 2, "trains": 4, "stops": 2 * (7 + 14)}


def test_sync_operations_reads_every_page() -> None:
    summary, _, repo = run(pipelines.sync_operations)

    assert summary == {"operations": 2, "stops": 28}
    [(name, (batch,))] = repo.calls
    assert name == "save_operations"
    assert len(batch.operations) == 2


def test_sync_exports_its_spans_before_returning_and_on_failure() -> None:
    exporter = InMemorySpanExporter()
    provider = TracerProvider()
    # A delay no test waits out: spans only reach the exporter through force_flush.
    provider.add_span_processor(BatchSpanProcessor(exporter, schedule_delay_millis=3_600_000))
    plk, repo = cast(PlkClient, FakePlk()), cast(Repository, RecordingRepository())

    with (
        patch("opentelemetry.trace.get_tracer_provider", return_value=provider),
        patch("pociag_processing.tracing.get_tracer", return_value=provider.get_tracer("test")),
    ):
        pipelines.sync_operations(client=plk, repository=repo)
        assert [s.name for s in exporter.get_finished_spans()] == ["operations.sync"]

        with pytest.raises(RuntimeError):
            pipelines.sync_operations(client=plk, repository=cast(Repository, BrokenRepository()))
        assert [s.name for s in exporter.get_finished_spans()][-1] == "operations.sync"
        assert len(exporter.get_finished_spans()) == 2
    provider.shutdown()


def test_sync_disruptions_covers_today_and_tomorrow() -> None:
    summary, plk, repo = run(pipelines.sync_disruptions, date(2026, 10, 4))

    assert plk.disruption_range == (date(2026, 10, 4), date(2026, 10, 5))
    assert summary == {"disruptions": 3}
    assert [name for name, _ in repo.calls] == ["replace_disruptions"]
