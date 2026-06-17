from __future__ import annotations

import json
from datetime import date
from typing import Any

import boto3
from opentelemetry import trace


class LakeReader:
    def __init__(self, endpoint: str, bucket: str, access_key: str, secret_key: str) -> None:
        self._bucket = bucket
        self._tracer = trace.get_tracer("pociag.processor")
        self._client = boto3.client(
            "s3",
            endpoint_url=endpoint,
            aws_access_key_id=access_key,
            aws_secret_access_key=secret_key,
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

    def count_raw_dictionaries(self, run_id: int | None, run_date: date) -> int:
        with self._tracer.start_as_current_span("lake.dictionaries.count"):
            objects = self.read_raw_dictionaries(run_id, run_date)
            return sum(
                int(obj.get("metadata", {}).get("record_count", 0)) for obj in objects
            )

    def count_raw_schedules(
        self, date_from: date, date_to: date, run_id: int | None
    ) -> int:
        with self._tracer.start_as_current_span("lake.schedules.count"):
            objects = self.read_raw_schedules(date_from, date_to, run_id)
            return sum(
                int(obj.get("metadata", {}).get("record_count", 0)) for obj in objects
            )

    def count_raw_operations(self, operating_date: date, run_id: int | None) -> int:
        with self._tracer.start_as_current_span("lake.operations.count"):
            objects = self.read_raw_operations(operating_date, run_id)
            return sum(
                int(obj.get("metadata", {}).get("record_count", 0)) for obj in objects
            )

    def count_raw_disruptions(
        self, date_from: date, date_to: date, run_id: int | None
    ) -> int:
        with self._tracer.start_as_current_span("lake.disruptions.count"):
            objects = self.read_raw_disruptions(date_from, date_to, run_id)
            return sum(
                int(obj.get("metadata", {}).get("record_count", 0)) for obj in objects
            )

    def _read_objects(self, prefix: str) -> list[dict[str, Any]]:
        results: list[dict[str, Any]] = []
        paginator = self._client.get_paginator("list_objects_v2")
        for page in paginator.paginate(Bucket=self._bucket, Prefix=prefix):
            for obj in page.get("Contents", []):
                key = obj["Key"]
                response = self._client.get_object(Bucket=self._bucket, Key=key)
                body = response["Body"].read()
                try:
                    envelope = json.loads(body)
                    results.append(envelope)
                except (json.JSONDecodeError, ValueError):
                    continue
        return results
