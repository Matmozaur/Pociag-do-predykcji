"""PLK Open Data API client. Base URL and API key come from the ``pociag_plk`` connection."""

from __future__ import annotations

from collections.abc import Callable, Iterator
from datetime import date
from typing import Any, cast

import httpx
from airflow.hooks.base import BaseHook

from pociag_processing.tracing import get_tracer

OPERATIONS_PAGE_SIZE = 5000  # PLK maximum
STATIONS_PAGE_SIZE = 10000  # PLK maximum


class PlkClient:
    def __init__(self, conn_id: str = "pociag_plk", timeout: float = 180.0) -> None:
        conn = BaseHook.get_connection(conn_id)
        host = (conn.host or "").strip().rstrip("/")
        base_url = host if "://" in host else f"{conn.schema or 'https'}://{host}"
        self._http = httpx.Client(
            base_url=base_url,
            headers={"Accept": "application/json", "X-API-Key": (conn.password or "").strip()},
            timeout=timeout,
        )
        self._tracer = get_tracer()

    def get(self, path: str, **params: str | int) -> dict[str, Any]:
        with self._tracer.start_as_current_span("plk.get") as span:
            span.set_attribute("http.route", path)
            response = self._http.get(path, params=params)
            response.raise_for_status()
            return cast(dict[str, Any], response.json())

    def stations(self) -> Iterator[dict[str, Any]]:
        yield from self._pages(
            "/api/v1/dictionaries/stations", lambda p: int(p.get("totalPages") or 1),
            pageSize=STATIONS_PAGE_SIZE,
        )

    def cities(self) -> dict[str, Any]:
        return self.get("/api/v1/dictionaries/cities")

    def schedules(self, day: date) -> dict[str, Any]:
        """Every train running on ``day``, with full routes and the station/carrier dictionaries."""
        return self.get("/api/v1/schedules", dateFrom=day.isoformat(), dateTo=day.isoformat())

    def operations(self) -> Iterator[dict[str, Any]]:
        """The current operations snapshot (today's and tomorrow's runs), page by page."""
        yield from self._pages(
            "/api/v1/operations",
            lambda p: int((p.get("pagination") or {}).get("totalPages") or 1),
            pageSize=OPERATIONS_PAGE_SIZE,
            withPlanned="true",
        )

    def disruptions(self, date_from: date, date_to: date) -> dict[str, Any]:
        return self.get(
            "/api/v1/disruptions", dateFrom=date_from.isoformat(), dateTo=date_to.isoformat()
        )

    def _pages(
        self, path: str, total_pages: Callable[[dict[str, Any]], int], **params: str | int
    ) -> Iterator[dict[str, Any]]:
        page = 1
        while True:
            payload = self.get(path, page=page, **params)
            yield payload
            if page >= total_pages(payload):
                return
            page += 1
