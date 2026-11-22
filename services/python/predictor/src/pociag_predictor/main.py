"""Entry point: ``uvicorn --factory pociag_predictor.main:build_app``."""

from __future__ import annotations

import logging
import os

from fastapi import FastAPI

from pociag_predictor.app import create_app
from pociag_predictor.repository import Repository
from pociag_predictor.tracing import setup_tracing


def build_app() -> FastAPI:
    logging.basicConfig(level=logging.INFO)
    dsn = os.environ.get("DATABASE_DSN")
    if not dsn:
        raise RuntimeError("DATABASE_DSN is required")
    app = create_app(Repository(dsn))
    setup_tracing(app)
    return app
