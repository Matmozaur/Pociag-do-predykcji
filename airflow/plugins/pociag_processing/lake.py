from __future__ import annotations

import json
from datetime import date
from typing import Any, cast

import boto3  # type: ignore[import-untyped]
from airflow.hooks.base import BaseHook

from pociag_processing.tracing import get_tracer


class LakeReader:
    def __init__(self, conn_id: str = "pociag_s3") -> None:
        conn = BaseHook.get_connection(conn_id)
        extras: dict[str, Any] = conn.extra_dejson
        endpoint_url: str = extras.get("endpoint_url", "")
        bucket: str = extras.get("bucket", "pociag-lake")

        self._bucket = bucket
        self._tracer = get_tracer()
        self._client: Any = cast(Any, boto3).client(
            "s3",
            endpoint_url=endpoint_url,
            aws_access_key_id=conn.login,
            aws_secret_access_key=conn.password,
            region_name="us-east-1",
        )

    def read_raw_dictionaries(self, run_id: int | None, run_date: date) -> list[dict[str, Any]]:
        with self._tracer.start_as_current_span("lake.dictionaries.read"):
            prefix = (
                f"raw/dictionaries/{run_date.year:04d}"
                f"/{run_date.month:02d}/{run_date.day:02d}/"
            )
            if run_id is not None:
                prefix += f"run_{run_id}_"
            return self._read_objects(prefix)

    def read_raw_schedules(
        self, date_from: date, date_to: date, run_id: int | None
    ) -> list[dict[str, Any]]:
        with self._tracer.start_as_current_span("lake.schedules.read"):
            prefix = (
                f"raw/schedules/{date_from.year:04d}/{date_from.month:02d}/{date_from.day:02d}/"
            )
            if run_id is not None:
                prefix += f"run_{run_id}_"
            return self._read_objects(prefix)

    def read_raw_operations(self, operating_date: date, run_id: int | None) -> list[dict[str, Any]]:
        with self._tracer.start_as_current_span("lake.operations.read"):
            prefix = (
                f"raw/operations/{operating_date.year:04d}"
                f"/{operating_date.month:02d}/{operating_date.day:02d}/"
            )
            if run_id is not None:
                prefix += f"run_{run_id}_"
            return self._read_objects(prefix)

    def read_raw_disruptions(
        self, date_from: date, date_to: date, run_id: int | None
    ) -> list[dict[str, Any]]:
        with self._tracer.start_as_current_span("lake.disruptions.read"):
            prefix = (
                f"raw/disruptions/{date_from.year:04d}/{date_from.month:02d}/{date_from.day:02d}/"
            )
            if run_id is not None:
                prefix += f"run_{run_id}_"
            return self._read_objects(prefix)

    def _read_objects(self, prefix: str) -> list[dict[str, Any]]:
        results: list[dict[str, Any]] = []
        paginator = self._client.get_paginator("list_objects_v2")
        for page in paginator.paginate(Bucket=self._bucket, Prefix=prefix):
            for obj in page.get("Contents", []):
                key = obj["Key"]
                response = self._client.get_object(Bucket=self._bucket, Key=key)
                stream = response["Body"]
                try:
                    body = stream.read()
                finally:
                    stream.close()
                try:
                    envelope = json.loads(body)
                    results.append(envelope)
                    import logging
                    logging.getLogger("pociag_processing.lake").warning(
                        "Failed to parse object: %s", key
                    )
                    continue
        return results
