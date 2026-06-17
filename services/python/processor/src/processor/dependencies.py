from functools import lru_cache
from typing import cast

from fastapi import Request
from opentelemetry import trace
from opentelemetry.trace import Tracer

from processor.config import Settings
from processor.lake import LakeReader
from processor.repository import Repository


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    return Settings()


def get_repository(request: Request) -> Repository:
    return cast(Repository, request.app.state.repository)


def get_lake_reader(request: Request) -> LakeReader:
    return cast(LakeReader, request.app.state.lake_reader)


def get_tracer() -> Tracer:
    return trace.get_tracer("pociag.processor")
