from __future__ import annotations

import math
from collections.abc import Iterator
from datetime import UTC, date, datetime

import pytest
from fastapi.testclient import TestClient

from pociag_predictor.app import create_app

# 00:30 Warsaw on 5 October is still 4 October in UTC: the operating date is the Warsaw one.
NOW = datetime(2026, 10, 4, 22, 30, tzinfo=UTC)


class FakeStore:
    def __init__(
        self, delays: list[int] | None = None, ping_error: Exception | None = None
    ) -> None:
        self.delays = delays or []
        self.ping_error = ping_error
        self.asked: tuple[int, int, date, date] | None = None
        self.closed = False

    def ping(self) -> None:
        if self.ping_error:
            raise self.ping_error

    def close(self) -> None:
        self.closed = True

    def arrival_delays(
        self, train_id: int, station_id: int, first_day: date, last_day: date
    ) -> list[int]:
        self.asked = (train_id, station_id, first_day, last_day)
        return self.delays


def client_for(store: FakeStore) -> Iterator[TestClient]:
    with TestClient(create_app(store, clock=lambda: NOW)) as client:
        yield client


@pytest.fixture
def store() -> FakeStore:
    return FakeStore(delays=[0, 2, 4, 12])


@pytest.fixture
def client(store: FakeStore) -> Iterator[TestClient]:
    yield from client_for(store)


def test_predict_delay_from_history(client: TestClient, store: FakeStore) -> None:
    response = client.post("/v1/predict-delay", json={"train_id": 3, "to_station_id": 4900})

    assert response.status_code == 200
    body = response.json()
    assert [b["range"] for b in body["buckets"]] == [
        "0-5", "5-10", "10-20", "20-40", "40-60", "60-120", "120+",
    ]  # fmt: skip
    assert [b["probability"] for b in body["buckets"]] == [0.75, 0, 0.25, 0, 0, 0, 0]
    assert body["model_version"] == "history-7d"
    assert body["is_mock"] is False
    # Seven operating dates ending today, today being the Warsaw date.
    assert store.asked == (3, 4900, date(2026, 9, 29), date(2026, 10, 5))


def test_predict_delay_without_history_is_a_placeholder() -> None:
    for client in client_for(FakeStore()):
        body = client.post("/v1/predict-delay", json={"train_id": 3, "to_station_id": 4900}).json()

        assert body["is_mock"] is True
        assert math.isclose(sum(b["probability"] for b in body["buckets"]), 1)


@pytest.mark.parametrize(
    "payload",
    [
        {},
        {"train_id": 3},
        {"train_id": 0, "to_station_id": 1},
        {"train_id": 3, "to_station_id": "x"},
    ],
)
def test_predict_delay_rejects_invalid_requests(
    client: TestClient, payload: dict[str, object]
) -> None:
    assert client.post("/v1/predict-delay", json=payload).status_code == 422


def test_probes(client: TestClient) -> None:
    assert client.get("/healthz").status_code == 200
    assert client.get("/readyz").status_code == 200


def test_not_ready_when_the_database_is_down() -> None:
    for client in client_for(FakeStore(ping_error=RuntimeError("down"))):
        assert client.get("/readyz").status_code == 503
        assert client.get("/healthz").status_code == 200


def test_store_is_closed_on_shutdown(store: FakeStore) -> None:
    for _ in client_for(store):
        assert not store.closed
    assert store.closed
