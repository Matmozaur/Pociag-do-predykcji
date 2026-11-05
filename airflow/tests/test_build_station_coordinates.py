from __future__ import annotations

import importlib.util
import sys
from pathlib import Path
from types import ModuleType
from typing import Any

import pytest

_SCRIPT = Path(__file__).resolve().parent.parent / "scripts" / "build_station_coordinates.py"


def _load_script() -> ModuleType:
    spec = importlib.util.spec_from_file_location("build_station_coordinates", _SCRIPT)
    assert spec is not None and spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    sys.modules[spec.name] = module  # dataclasses resolve annotations via sys.modules
    spec.loader.exec_module(module)
    return module


bsc = _load_script()


def _osm(osm_id: str, name: str, lat: float, lon: float, **kw: Any) -> Any:
    names = kw.pop("names", (name,))
    return bsc.OsmStation(osm_id, name, lat, lon, names=names, primary_names=(name,), **kw)


# --------------------------------------------------------------------------- normalisation


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        ("Łódź Kaliska", "lodz kaliska"),
        ("Kraków Główny", "krakow gl"),
        ("Kraków Gł.", "krakow gl"),
        ("Żurawica Osobowa", "zurawica os"),
        ("Żurawica Os.", "zurawica os"),
        ("Bielsko-Biała  Lipnik", "bielsko biala lipnik"),
        ("Św.Maksymiliana", "sw maksymiliana"),
        ("Grodzisk Wielkopolski", "grodzisk wlkp"),
        ("Grodzisk Wlkp.", "grodzisk wlkp"),
        ("Gdańsk Wrzeszcz", "gdansk wrzeszcz"),
    ],
)
def test_normalise_name(raw: str, expected: str) -> None:
    assert bsc.normalise_name(raw) == expected


def test_base_name_strips_suffixes_but_not_everything() -> None:
    assert bsc.base_name("Szczecin Główny") == "szczecin"
    assert bsc.base_name("Kostrzyn Os.") == "kostrzyn"
    assert bsc.base_name("Warszawa Raków WKD") == "warszawa rakow"
    assert bsc.base_name("Os") == "os"


def test_strip_diacritics_handles_polish_l() -> None:
    assert bsc.strip_diacritics("Łęczyca Ślężna") == "Leczyca Slezna"


def test_town_key() -> None:
    assert bsc.town_key(bsc.PlkStation(1, "Radom Południowy")) == "radom"
    assert bsc.town_key(bsc.PlkStation(1, "Zwierzyniec")) == ""
    assert bsc.town_key(bsc.PlkStation(1, "Rumia Janowo", "RUMIA")) == "rumia"


def test_is_placeholder() -> None:
    assert bsc.is_placeholder("WARSZAWA -")
    assert bsc.is_placeholder("MŁAWA-")
    assert not bsc.is_placeholder("Kędzierzyn-Koźle")


# --------------------------------------------------------------------------- OSM parsing


def test_parse_overpass_reads_nodes_and_way_centres_and_skips_subway() -> None:
    payload = {
        "elements": [
            {"type": "node", "id": 1, "lat": 52.0, "lon": 21.0, "tags": {"name": "A"}},
            {
                "type": "way",
                "id": 2,
                "center": {"lat": 50.0, "lon": 19.0},
                "tags": {"name": "B", "alt_name": "B1;B2", "addr:city": "Kraków"},
            },
            {"type": "node", "id": 3, "lat": 52.2, "lon": 21.0,
             "tags": {"name": "Metro", "station": "subway"}},
            {"type": "node", "id": 4, "lat": 52.2, "lon": 21.0, "tags": {}},
        ]
    }
    result = bsc.parse_overpass(payload)
    assert [r.osm_id for r in result] == ["node/1", "way/2"]
    assert result[1].latitude == 50.0
    assert result[1].names == ("B", "B1", "B2")
    assert result[1].primary_names == ("B",)
    assert result[1].city == "Kraków"


# --------------------------------------------------------------------------- matching


def test_collapse_duplicates_prefers_node_within_radius() -> None:
    way = _osm("way/9", "X", 52.0, 21.0)
    node = _osm("node/5", "X", 52.001, 21.001)
    assert bsc.collapse_duplicates([way, node]) is node
    far = _osm("node/6", "X", 50.0, 19.0)
    assert bsc.collapse_duplicates([node, far]) is None


def test_resolve_candidates_breaks_tie_by_city() -> None:
    station = bsc.PlkStation(1, "Lipie", "Kraków")
    a = _osm("node/1", "Lipie", 52.0, 21.0, city="Warszawa")
    b = _osm("node/2", "Lipie", 50.0, 19.9, city="Kraków")
    assert bsc.resolve_candidates(station, [a, b]) == (b, "city")


def test_resolve_candidates_breaks_tie_by_nearest_anchor() -> None:
    station = bsc.PlkStation(1, "Radom Wschodni")
    near = _osm("node/1", "Radom Wschodni", 51.40, 21.16)
    far = _osm("node/2", "Radom Wschodni", 53.0, 18.0)
    anchor = _osm("node/3", "Radom", 51.40, 21.15)
    assert bsc.resolve_candidates(station, [near, far], [anchor]) == (near, "nearest")
    assert bsc.resolve_candidates(station, [near, far]) == (None, "ambiguous")


def test_match_stations_tiers_suffixes_and_ambiguity() -> None:
    osm = [
        _osm("node/1", "Szczecin Główny", 53.418, 14.549),
        _osm("node/2", "Szczecin Dąbie", 53.394, 14.668),
        _osm("node/3", "Witkowo", 52.44, 17.77),
        _osm("node/4", "Witkowo", 53.28, 15.08),
        # "Trablice" only carries the PLK name as old_name: a primary match must win.
        _osm("node/5", "Trablice", 51.35, 21.10, names=("Trablice", "Radom Południowy")),
        _osm("node/6", "Radom Południowy", 51.363, 21.137),
    ]
    plk = [
        bsc.PlkStation(10, "Szczecin Gł."),
        bsc.PlkStation(11, "Szczecin Dąbie"),
        bsc.PlkStation(12, "Witkowo"),
        bsc.PlkStation(13, "Radom Południowy"),
        bsc.PlkStation(14, "Nowhere"),
        bsc.PlkStation(15, "WARSZAWA -"),
    ]
    matches, unmatched = bsc.match_stations(plk, osm)
    by_id = {m.station.external_id: m.osm.osm_id for m in matches}
    assert by_id == {10: "node/1", 11: "node/2", 13: "node/6"}
    reasons = {s.external_id: why for s, why in unmatched}
    assert reasons[12].startswith("ambiguous")
    assert reasons[14] == "no-candidate"
    assert reasons[15] == "placeholder"


def test_match_stations_skips_foreign_homonyms() -> None:
    osm = [_osm("node/1", "Kolin", 53.245, 15.124)]
    plk = [bsc.PlkStation(2329, "Kolin"), bsc.PlkStation(128006, "Kolin")]
    matches, unmatched = bsc.match_stations(plk, osm)
    assert [m.station.external_id for m in matches] == [2329]
    assert unmatched[0][1].startswith("abroad")


def test_build_payload_keeps_curated_entries_first_and_unchanged() -> None:
    curated = [{"external_id": 1, "name": "Curated", "latitude": 1.0, "longitude": 2.0}]
    osm_a = _osm("node/1", "A", 52.123456, 21.654321)
    matches = [
        bsc.Match(bsc.PlkStation(1, "Curated"), osm_a, "name:unique"),
        bsc.Match(bsc.PlkStation(3, "C"), osm_a, "name:unique"),
        bsc.Match(bsc.PlkStation(2, "B"), osm_a, "name:unique"),
    ]
    payload = bsc.build_payload(curated, matches)
    assert "© OpenStreetMap contributors, ODbL" in payload["source"]
    assert payload["stations"][0] == curated[0]
    assert [s["external_id"] for s in payload["stations"]] == [1, 2, 3]
    assert payload["stations"][1] == {
        "external_id": 2, "name": "B", "latitude": 52.12346, "longitude": 21.65432,
    }


@pytest.mark.parametrize(
    ("latitude", "longitude", "expected"),
    [
        (51.249, 22.532, True),
        (53.0, 18.598, True),
        (52.2288, 21.003, True),  # hand-typed, one value has 3 decimals
        (52.2519, 21.0512, False),  # hand-typed, but both values have 4 decimals
        (52.12345, 18.123, False),  # round(x, 5) dropped the trailing zeros of one value
    ],
)
def test_is_low_precision_ignores_trailing_zeros_of_osm_values(
    latitude: float, longitude: float, expected: bool
) -> None:
    assert bsc.is_low_precision({"latitude": latitude, "longitude": longitude}) is expected


def test_refine_low_precision_replaces_only_low_precision_matches_in_place() -> None:
    low = {"external_id": 1, "name": "Low", "latitude": 51.249, "longitude": 22.532}
    precise = {"external_id": 2, "name": "Precise", "latitude": 52.2288, "longitude": 21.0512}
    osm = _osm("node/1", "Low OSM", 51.2301234, 22.5401234)
    matches = [
        bsc.Match(bsc.PlkStation(1, "Low"), osm, "name:unique"),
        bsc.Match(bsc.PlkStation(2, "Precise"), osm, "name:unique"),
    ]
    entries, refined, kept = bsc.refine_low_precision([low, precise], matches)
    expected = {"external_id": 1, "name": "Low", "latitude": 51.23012, "longitude": 22.54012}
    assert entries == [expected, precise]
    assert refined == [expected]
    assert kept == []


def test_refine_low_precision_keeps_unmatched_and_distant_entries() -> None:
    unmatched = {"external_id": 1, "name": "Unmatched", "latitude": 51.249, "longitude": 22.532}
    distant = {"external_id": 2, "name": "Distant", "latitude": 51.249, "longitude": 22.532}
    far_osm = _osm("node/9", "Distant", 52.5, 22.532)
    matches = [bsc.Match(bsc.PlkStation(2, "Distant"), far_osm, "name:unique")]
    entries, refined, kept = bsc.refine_low_precision([unmatched, distant], matches)
    assert entries == [unmatched, distant]
    assert refined == []
    assert [(e["external_id"], why.split(" ")[0]) for e, why in kept] == [
        (1, "no-osm-match"),
        (2, "too-far"),
    ]
    assert "node/9" in kept[1][1]


def test_refine_low_precision_is_idempotent() -> None:
    low = {"external_id": 1, "name": "Low", "latitude": 51.249, "longitude": 22.532}
    osm = _osm("node/1", "Low", 51.2301234, 22.5401234)
    matches = [bsc.Match(bsc.PlkStation(1, "Low"), osm, "name:unique")]
    once, _, _ = bsc.refine_low_precision([low], matches)
    twice, refined, kept = bsc.refine_low_precision(once, matches)
    assert twice == once
    assert refined == []
    assert kept == []
# --------------------------------------------------------------------------- filling gaps


def _curated(external_id: int, lat: float, lon: float) -> dict[str, Any]:
    return {"external_id": external_id, "name": "N", "latitude": lat, "longitude": lon}


def test_fill_keeps_a_unique_match_and_skips_stations_with_coordinates() -> None:
    plk = [bsc.PlkStation(1, "Kolumna"), bsc.PlkStation(2, "Skopanie")]
    osm = [_osm("node/1", "Kolumna", 51.6, 19.2), _osm("node/2", "Skopanie", 50.5, 21.6)]
    matches, unmatched = bsc.fill_missing_coordinates(plk, osm, [_curated(2, 50.0, 20.0)], [])
    assert [(m.station.external_id, m.method) for m in matches] == [(1, "name:unique")]
    assert unmatched == []


def test_fill_rejects_a_unique_match_far_from_route_neighbours() -> None:
    plk = [bsc.PlkStation(1, "Krasne")]
    osm = [_osm("node/1", "Krasne", 52.9, 20.6)]
    matches, unmatched = bsc.fill_missing_coordinates(plk, osm, [_curated(9, 50.0, 24.0)], [(1, 9)])
    assert matches == []
    assert unmatched == [(plk[0], "far from route neighbours (node/1)")]


def test_fill_breaks_a_name_tie_by_route_neighbours() -> None:
    plk = [bsc.PlkStation(1, "Majdan")]
    osm = [_osm("node/1", "Majdan", 49.206, 22.299), _osm("node/2", "Majdan", 51.148, 22.426)]
    curated = [_curated(9, 51.17, 22.45)]
    matches, unmatched = bsc.fill_missing_coordinates(plk, osm, curated, [(9, 1)])
    assert [(m.osm.osm_id, m.method) for m in matches] == [("node/2", "name:route")]
    assert unmatched == []


def test_fill_leaves_a_name_tie_without_neighbours_or_margin() -> None:
    osm = [_osm("node/1", "Otok", 52.0, 20.0), _osm("node/2", "Otok", 52.0, 20.3)]
    no_neighbours = bsc.PlkStation(1, "Otok")
    # These two lie 20 km apart, so they are not collapsed into one station.
    matches, unmatched = bsc.fill_missing_coordinates([no_neighbours], osm, [], [])
    assert matches == []
    assert unmatched[0][1].startswith("ambiguous")
    # Both candidates lie next to the neighbour: no margin, no match.
    near_both = bsc.PlkStation(2, "Otok")
    curated = [_curated(9, 52.0, 20.15)]
    matches, _ = bsc.fill_missing_coordinates([near_both], osm, curated, [(2, 9)])
    assert matches == []


def test_fill_drops_an_operator_suffix_only_for_one_place() -> None:
    plk = [bsc.PlkStation(1, "Gdańsk Oliwa SKM"), bsc.PlkStation(2, "Otok SKM")]
    osm = [
        _osm("node/1", "Gdańsk Oliwa", 54.409, 18.572),
        _osm("node/2", "Otok", 52.0, 20.0),
        _osm("node/3", "Otok", 50.0, 20.0),
    ]
    matches, unmatched = bsc.fill_missing_coordinates(plk, osm, [], [])
    assert [(m.station.external_id, m.method) for m in matches] == [(1, "operator-suffix")]
    assert [(s.external_id, why) for s, why in unmatched] == [(2, "no-candidate")]


def test_fill_adds_a_regional_qualifier_only_with_timetable_evidence() -> None:
    osm = [
        _osm("node/1", "Międzybórz Sycowski", 51.401, 17.653),
        _osm("node/2", "Chmielów Zagumnie", 50.523, 21.686),
    ]
    near = bsc.PlkStation(1, "Międzybórz")
    curated = [_curated(9, 51.41, 17.68), _curated(8, 50.5, 21.7)]
    matches, _ = bsc.fill_missing_coordinates([near], osm, curated, [(1, 9)])
    assert [(m.osm.osm_id, m.method) for m in matches] == [("node/1", "qualifier:route")]
    # No neighbours: a bare name is not enough.
    matches, unmatched = bsc.fill_missing_coordinates([near], osm, curated, [])
    assert matches == [] and unmatched == [(near, "no-candidate")]
    # A district name is a different halt, not a qualifier, even right next to the neighbour.
    other = bsc.PlkStation(2, "Chmielów")
    matches, _ = bsc.fill_missing_coordinates([other], osm, curated, [(2, 8)])
    assert matches == []
