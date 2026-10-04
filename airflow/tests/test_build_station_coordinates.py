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
