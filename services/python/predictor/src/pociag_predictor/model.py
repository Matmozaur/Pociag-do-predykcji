"""The delay ranges and how a list of observed arrival delays becomes a distribution over them.

Pure: no I/O, so every rule here is covered by plain unit tests.
"""

from __future__ import annotations

from collections.abc import Sequence
from dataclasses import dataclass

# Arrival delay ranges (min, max] in minutes, in the order the api exposes them. The first one
# also holds early and on-time arrivals, the last one is open-ended.
RANGES = ("0-5", "5-10", "10-20", "20-40", "40-60", "60-120", "120+")
_UPPER_BOUNDS = (5, 10, 20, 40, 60, 120)

HISTORY_MODEL_VERSION = "history-7d"
PLACEHOLDER_MODEL_VERSION = "placeholder-0"

# Mostly on time, with a tail: what a route without any recorded arrival gets.
_PLACEHOLDER = (0.70, 0.13, 0.08, 0.05, 0.02, 0.015, 0.005)


@dataclass(frozen=True)
class Prediction:
    probabilities: tuple[float, ...]  # one per RANGES entry, summing to 1
    model_version: str
    is_mock: bool


def range_index(delay: int) -> int:
    """The RANGES entry an arrival delay in minutes falls in."""
    for i, upper in enumerate(_UPPER_BOUNDS):
        if delay <= upper:
            return i
    return len(RANGES) - 1


def from_history(delays: Sequence[int]) -> Prediction:
    """Each range's probability is the share of ``delays`` in it; a placeholder without any."""
    if not delays:
        return Prediction(_PLACEHOLDER, PLACEHOLDER_MODEL_VERSION, is_mock=True)
    counts = [0] * len(RANGES)
    for delay in delays:
        counts[range_index(delay)] += 1
    return Prediction(
        tuple(count / len(delays) for count in counts), HISTORY_MODEL_VERSION, is_mock=False
    )
