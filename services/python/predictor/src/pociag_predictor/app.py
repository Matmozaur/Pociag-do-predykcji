"""The HTTP API, the contract is specs/openapi/predictor.yml."""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager
from datetime import UTC, datetime
from typing import Protocol

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field

from pociag_predictor.model import RANGES
from pociag_predictor.predictor import ArrivalHistory, predict
from pociag_predictor.tracing import get_tracer

logger = logging.getLogger("pociag.predictor")


class Store(ArrivalHistory, Protocol):
    def ping(self) -> None: ...

    def close(self) -> None: ...


class PredictRequest(BaseModel):
    train_id: int = Field(ge=1)
    to_station_id: int = Field(ge=1)


class RangeProbability(BaseModel):
    range: str
    probability: float


class PredictResponse(BaseModel):
    buckets: list[RangeProbability]
    model_version: str
    is_mock: bool


def _utcnow() -> datetime:
    return datetime.now(UTC)


def create_app(store: Store, clock: Callable[[], datetime] = _utcnow) -> FastAPI:
    @asynccontextmanager
    async def lifespan(_: FastAPI) -> AsyncIterator[None]:
        yield
        store.close()

    app = FastAPI(title="pociag predictor", lifespan=lifespan)

    @app.get("/healthz")
    def healthz() -> dict[str, str]:
        return {"status": "ok"}

    @app.get("/readyz")
    def readyz() -> dict[str, str]:
        try:
            store.ping()
        except Exception:
            logger.warning("not ready: database ping failed", exc_info=True)
            raise HTTPException(status_code=503, detail="database unreachable") from None
        return {"status": "ready"}

    # A plain def: FastAPI runs it in a worker thread, where the blocking query belongs.
    @app.post("/v1/predict-delay")
    def predict_delay(request: PredictRequest) -> PredictResponse:
        with get_tracer().start_as_current_span("delay.predict"):
            prediction = predict(store, request.train_id, request.to_station_id, clock())
        return PredictResponse(
            buckets=[
                RangeProbability(range=name, probability=p)
                for name, p in zip(RANGES, prediction.probabilities, strict=True)
            ],
            model_version=prediction.model_version,
            is_mock=prediction.is_mock,
        )

    return app
