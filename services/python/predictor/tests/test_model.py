from __future__ import annotations

import math

import pytest

from pociag_predictor.model import (
    HISTORY_MODEL_VERSION,
    RANGES,
    from_history,
    range_index,
)


@pytest.mark.parametrize(
    ("delay", "expected"),
    [
        (-30, "0-5"),  # early counts as on time
        (0, "0-5"),
        (5, "0-5"),  # ranges are (min, max]
        (6, "5-10"),
        (10, "5-10"),
        (11, "10-20"),
        (20, "10-20"),
        (21, "20-40"),
        (40, "20-40"),
        (41, "40-60"),
        (60, "40-60"),
        (61, "60-120"),
        (120, "60-120"),
        (121, "120+"),
        (900, "120+"),
    ],
)
def test_range_index(delay: int, expected: str) -> None:
    assert RANGES[range_index(delay)] == expected


def test_from_history_is_the_share_per_range() -> None:
    prediction = from_history([-3, 0, 5, 0, 6, 10, 11, 20, 21, 40, 41, 60, 61, 120, 121, 300])

    assert prediction.probabilities == (4 / 16, 2 / 16, 2 / 16, 2 / 16, 2 / 16, 2 / 16, 2 / 16)
    assert prediction.model_version == HISTORY_MODEL_VERSION
    assert not prediction.is_mock


def test_from_history_single_range() -> None:
    assert from_history([2, 3]).probabilities == (1, 0, 0, 0, 0, 0, 0)


def test_without_history_a_placeholder_distribution() -> None:
    prediction = from_history([])

    assert prediction.is_mock
    assert len(prediction.probabilities) == len(RANGES)
    assert math.isclose(sum(prediction.probabilities), 1)
