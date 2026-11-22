"""Predicts the arrival delay of a train at a stop from the train's recent history."""

from __future__ import annotations

from datetime import date, datetime, timedelta
from typing import Protocol
from zoneinfo import ZoneInfo

from pociag_predictor.model import Prediction, from_history

HISTORY_DAYS = 7  # operating dates looked back over, today included
_WARSAW = ZoneInfo("Europe/Warsaw")


class ArrivalHistory(Protocol):
    def arrival_delays(
        self, train_id: int, station_id: int, first_day: date, last_day: date
    ) -> list[int]:
        """Delay minutes of every confirmed, not cancelled arrival of the train at the station on
        the operating dates ``first_day``..``last_day`` (both inclusive)."""
        ...


def operating_date(now: datetime) -> date:
    """The PLK operating date: the Europe/Warsaw calendar date."""
    return now.astimezone(_WARSAW).date()


def predict(history: ArrivalHistory, train_id: int, station_id: int, now: datetime) -> Prediction:
    today = operating_date(now)
    delays = history.arrival_delays(
        train_id, station_id, today - timedelta(days=HISTORY_DAYS - 1), today
    )
    return from_history(delays)
