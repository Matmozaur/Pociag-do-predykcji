"""Regenerate ``pociag_processing/data/station_coordinates.json`` from OpenStreetMap.

One-off, offline helper. It is not part of the ``pociag_processing`` package and nothing imports
it at runtime. It uses only the standard library.

Usage (from ``airflow/``)::

    # export the PLK station list from the curated DB (host port 5434)
    psql -h localhost -p 5434 -U pociag -d pociag \\
        -c "\\copy (SELECT id AS external_id, name, city FROM stations) TO 'stations.csv' CSV HEADER"
    python scripts/build_station_coordinates.py stations.csv [--active-ids ids.txt]

The script:

* reads the PLK station list (CSV with ``external_id,name[,city]`` or a JSON list of objects);
* queries Overpass for ``node|way[railway~"^(station|halt)$"]`` inside Poland;
* matches stations by normalised name: lowercase, no diacritics, punctuation collapsed, and
  common suffixes such as "Główny"/"Gł." and "Osobowa"/"Os." canonicalised or stripped;
* breaks ties with the PLK ``city``, by collapsing nearby duplicates, and by proximity to
  already-matched stations of the same town; if the tie remains, the station is left unmatched;
* replaces the coordinates of low-precision curated entries (see ``is_low_precision``) with their
  OSM match, unless the match is suspiciously far (see ``refine_low_precision``);
* writes the JSON in the existing ``{source, stations: [...]}`` format. Other entries already
  present in the curated file always win;
* prints a coverage report, then the unmatched stations.

Generated entries become part of the curated file. A later run therefore keeps them as they are.
To rebuild from scratch, pass ``--curated`` pointing at a file that holds only the hand-curated
entries.

With ``--adjacent-stops`` the script only fills stations that have no coordinates yet, using the
timetable as evidence (see ``fill_missing_coordinates``). Export the pairs from the curated DB::

    psql -h localhost -p 5434 -U pociag -d pociag -c "\\copy (SELECT DISTINCT station_id,
        next_station_id FROM (SELECT station_id, lead(station_id) OVER (PARTITION BY train_id
        ORDER BY seq) AS next_station_id FROM schedule_stops) s WHERE next_station_id IS NOT NULL
        AND next_station_id <> station_id) TO 'adjacent.csv' CSV HEADER"
    python scripts/build_station_coordinates.py stations.csv --adjacent-stops adjacent.csv
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import re
import sys
import unicodedata
import urllib.parse
import urllib.request
from collections import defaultdict
from collections.abc import Iterable, Sequence
from dataclasses import dataclass
from pathlib import Path
from typing import Any

DEFAULT_OUTPUT = (
    Path(__file__).resolve().parent.parent
    / "plugins"
    / "pociag_processing"
    / "data"
    / "station_coordinates.json"
)
DEFAULT_OVERPASS_URL = "https://overpass-api.de/api/interpreter"
OVERPASS_QUERY = (
    "[out:json][timeout:90];"
    'area["ISO3166-1"="PL"][admin_level=2]->.pl;'
    '(node["railway"~"^(station|halt)$"](area.pl);'
    'way["railway"~"^(station|halt)$"](area.pl););'
    "out center tags;"
)
SOURCE = (
    "Curated WGS84 coordinates for Polish railway stations, keyed by PLK external_id. "
    "Hand-curated entries plus coordinates matched by name from OpenStreetMap railway=station/halt "
    "(scripts/build_station_coordinates.py). Map data © OpenStreetMap contributors, ODbL "
    "(https://www.openstreetmap.org/copyright)."
)

# OSM name tags to match on, in priority order. ``alt_name``/``old_name`` may hold "a;b" lists.
# Primary tags are tried first; the rest are only consulted if no primary name matches.
_PRIMARY_NAME_TAGS = ("name", "name:pl")
_NAME_TAGS = (*_PRIMARY_NAME_TAGS, "official_name", "alt_name", "old_name", "short_name", "uic_name")
# OSM ``station=*`` values that are not mainline rail.
_EXCLUDED_STATION_KINDS = frozenset({"subway", "funicular", "light_rail", "monorail"})

# Word-level canonicalisation, applied to the normalised token stream. Abbreviations and full
# forms map to one token. Tokens in ``_STRIPPABLE`` are dropped for the looser base key.
_TOKEN_CANON: dict[str, str] = {
    "glowny": "gl",
    "glowna": "gl",
    "glowne": "gl",
    "gl": "gl",
    "osobowa": "os",
    "osobowy": "os",
    "os": "os",
    "polnocny": "pn",
    "polnocna": "pn",
    "polnocne": "pn",
    "pln": "pn",
    "pn": "pn",
    "poludniowy": "pd",
    "poludniowa": "pd",
    "poludniowe": "pd",
    "pd": "pd",
    "wschodni": "wsch",
    "wschodnia": "wsch",
    "wschodnie": "wsch",
    "wsch": "wsch",
    "zachodni": "zach",
    "zachodnia": "zach",
    "zachodnie": "zach",
    "zach": "zach",
    "przedmiescie": "przedm",
    "przedm": "przedm",
    "miasto": "m",
    "m": "m",
    "wielkopolski": "wlkp",
    "wielkopolska": "wlkp",
    "wielkopolskie": "wlkp",
    "wlkp": "wlkp",
    "mazowiecki": "maz",
    "mazowiecka": "maz",
    "mazowieckie": "maz",
    "maz": "maz",
    "slaski": "sl",
    "slaska": "sl",
    "slaskie": "sl",
    "sl": "sl",
    "pomorski": "pom",
    "pomorska": "pom",
    "pomorskie": "pom",
    "pom": "pom",
    "kolo": "k",
    "k": "k",
    "swiety": "sw",
    "swieta": "sw",
    "swietej": "sw",
    "swietego": "sw",
    "sw": "sw",
}
# "WKD" is an operator suffix PLK appends to Warszawska Kolej Dojazdowa halts.
_STRIPPABLE = frozenset({"gl", "os", "wkd"})

_NON_ALNUM = re.compile(r"[^a-z0-9]+")
# Letters NFKD does not decompose into base + combining mark.
_TRANSLIT = str.maketrans({"ł": "l", "Ł": "L", "ø": "o", "Ø": "O", "ß": "ss"})

# PLK stations abroad whose name also exists as a Polish OSM station. Only Poland is queried,
# so a name match for these would be wrong.
FOREIGN_HOMONYMS: dict[int, str] = {128006: "Kolín, Czechia (not Kolin, Zachodniopomorskie)"}

# Candidates closer than this are one physical station (node + way, duplicate tagging).
DUPLICATE_RADIUS_KM = 2.0
# A tie is broken by proximity only if the winner is this close to same-town matches...
TOWN_RADIUS_KM = 25.0
# ...and clearly closer than the runner-up.
TOWN_MARGIN_KM = 15.0
# A match for a station without coordinates must lie this close to a stop the timetable puts next
# to it. Express hops can be longer: those stations stay empty rather than risk a wrong location.
NEIGHBOUR_RADIUS_KM = 20.0
# ...and a same-named candidate elsewhere must be at least this much farther from those stops.
NEIGHBOUR_MARGIN_KM = 25.0
# Operator suffix PLK appends to one physical station ("Gdańsk Oliwa SKM").
_OPERATOR_SUFFIXES = frozenset({"skm"})
# Regional adjective OSM may add to a name PLK leaves bare ("Międzybórz" -> "Międzybórz Sycowski").
_REGIONAL_QUALIFIER = re.compile(r"(ski|ska|skie|cki|cka|ckie)$")

# Hand-typed entries are approximate and have at most 4 decimals, with a value of at most this many.
# One short value alone is not enough: ``round(x, 5)`` drops trailing zeros, so an OSM entry such as
# (52.12345, 18.12300) is precise. Its other value has the full ``GENERATED_DECIMALS``.
LOW_PRECISION_DECIMALS = 3
GENERATED_DECIMALS = 5
# A low-precision entry further than this from its OSM match is kept and reported, not replaced:
# hand-typed values are off by a few km, so a larger gap means a wrong name match.
MAX_REFINE_SHIFT_KM = 10.0


@dataclass(frozen=True)
class PlkStation:
    external_id: int
    name: str
    city: str | None = None


@dataclass(frozen=True)
class OsmStation:
    osm_id: str
    name: str
    latitude: float
    longitude: float
    names: tuple[str, ...] = ()
    city: str | None = None
    primary_names: tuple[str, ...] = ()


@dataclass(frozen=True)
class Match:
    station: PlkStation
    osm: OsmStation
    method: str


# --------------------------------------------------------------------------- normalisation


def strip_diacritics(text: str) -> str:
    decomposed = unicodedata.normalize("NFKD", text.translate(_TRANSLIT))
    return "".join(ch for ch in decomposed if not unicodedata.combining(ch))


def _tokens(name: str) -> list[str]:
    plain = _NON_ALNUM.sub(" ", strip_diacritics(name).lower())
    return [_TOKEN_CANON.get(tok, tok) for tok in plain.split()]


def normalise_name(name: str) -> str:
    """Return the strict match key: no diacritics or punctuation, abbreviations canonicalised."""
    return " ".join(_tokens(name))


def base_name(name: str) -> str:
    """Return the loose match key: like ``normalise_name`` but without Główny/Osobowa suffixes.

    A suffix is only dropped when something else remains, so "Os" alone stays "os".
    """
    tokens = _tokens(name)
    kept = [tok for tok in tokens if tok not in _STRIPPABLE]
    return " ".join(kept or tokens)


def town_key(station: PlkStation) -> str:
    """Return the station's town, or ``""`` if it cannot be told from the PLK data.

    Uses PLK ``city`` if set. Otherwise uses the first word of a multi-word name ("Radom
    Południowy" -> "radom"). A one-word name has no separate town, because using itself as the
    town would be circular.
    """
    if station.city:
        return normalise_name(station.city)
    tokens = normalise_name(station.name).split()
    return tokens[0] if len(tokens) > 1 else ""


def is_placeholder(name: str) -> bool:
    """Return True for PLK city-level pseudo stations such as "WARSZAWA -"."""
    return name.rstrip().endswith("-")


# --------------------------------------------------------------------------- geometry


def haversine_km(lat1: float, lon1: float, lat2: float, lon2: float) -> float:
    rlat1, rlat2 = math.radians(lat1), math.radians(lat2)
    dlat = rlat2 - rlat1
    dlon = math.radians(lon2 - lon1)
    a = math.sin(dlat / 2) ** 2 + math.cos(rlat1) * math.cos(rlat2) * math.sin(dlon / 2) ** 2
    return 6371.0 * 2 * math.asin(math.sqrt(a))


def _distance(a: OsmStation, b: OsmStation) -> float:
    return haversine_km(a.latitude, a.longitude, b.latitude, b.longitude)


def collapse_duplicates(candidates: Sequence[OsmStation]) -> OsmStation | None:
    """Return one representative if all candidates lie within ``DUPLICATE_RADIUS_KM``.

    Prefers nodes (the station point) over way centres. Returns ``None`` if the candidates are
    genuinely different places.
    """
    if not candidates:
        return None
    for i, a in enumerate(candidates):
        for b in candidates[i + 1 :]:
            if _distance(a, b) > DUPLICATE_RADIUS_KM:
                return None
    return sorted(candidates, key=lambda c: (not c.osm_id.startswith("node/"), c.osm_id))[0]


# --------------------------------------------------------------------------- OSM parsing


def parse_overpass(payload: dict[str, Any]) -> list[OsmStation]:
    """Turn an Overpass ``out center tags`` response into ``OsmStation`` records."""
    result: list[OsmStation] = []
    for el in payload.get("elements", []):
        tags: dict[str, str] = el.get("tags") or {}
        if tags.get("station") in _EXCLUDED_STATION_KINDS:
            continue
        if "lat" in el and "lon" in el:
            lat, lon = float(el["lat"]), float(el["lon"])
        elif isinstance(el.get("center"), dict):
            lat, lon = float(el["center"]["lat"]), float(el["center"]["lon"])
        else:
            continue
        names: list[str] = []
        primary: list[str] = []
        for tag in _NAME_TAGS:
            for value in (tags.get(tag) or "").split(";"):
                value = value.strip()
                if value and value not in names:
                    names.append(value)
                    if tag in _PRIMARY_NAME_TAGS:
                        primary.append(value)
        if not names:
            continue
        city = tags.get("addr:city") or tags.get("is_in:city") or None
        result.append(
            OsmStation(
                osm_id=f"{el.get('type', 'node')}/{el.get('id')}",
                name=names[0],
                latitude=lat,
                longitude=lon,
                names=tuple(names),
                city=city,
                primary_names=tuple(primary),
            )
        )
    return result


def fetch_overpass(url: str, timeout_s: float = 180.0) -> dict[str, Any]:
    data = urllib.parse.urlencode({"data": OVERPASS_QUERY}).encode()
    request = urllib.request.Request(
        url,
        data=data,
        headers={"User-Agent": "pociag-do-predykcji/build_station_coordinates"},
    )
    with urllib.request.urlopen(request, timeout=timeout_s) as response:
        payload: dict[str, Any] = json.loads(response.read().decode("utf-8"))
    return payload


# --------------------------------------------------------------------------- matching


Index = dict[str, list[OsmStation]]


def build_index(osm: Iterable[OsmStation]) -> list[tuple[str, Index]]:
    """Index OSM stations by match key, in the order the lookups are tried.

    The tiers are strict key on primary names, strict key on all names, loose (suffix-stripped)
    key on primary names, and loose key on all names. Each returned pair is
    ``(tier_label, index)``.
    """
    tiers: list[tuple[str, Index]] = [
        ("name", defaultdict(list)),
        ("name-alt", defaultdict(list)),
        ("base", defaultdict(list)),
        ("base-alt", defaultdict(list)),
    ]
    for st in osm:
        primary = st.primary_names or st.names[:1]
        for (_, index), keyfn, names in (
            (tiers[0], normalise_name, primary),
            (tiers[1], normalise_name, st.names),
            (tiers[2], base_name, primary),
            (tiers[3], base_name, st.names),
        ):
            for key in {keyfn(n) for n in names}:
                index[key].append(st)
    return tiers


def _plk_key(tier: str, name: str) -> str:
    return base_name(name) if tier.startswith("base") else normalise_name(name)


def _by_city(station: PlkStation, candidates: Sequence[OsmStation]) -> list[OsmStation]:
    if not station.city:
        return []
    wanted = normalise_name(station.city)
    return [c for c in candidates if c.city and normalise_name(c.city) == wanted]


def resolve_candidates(
    station: PlkStation,
    candidates: Sequence[OsmStation],
    anchors: Sequence[OsmStation] = (),
) -> tuple[OsmStation | None, str]:
    """Pick one OSM candidate for ``station`` or return ``(None, reason)``.

    ``anchors`` are OSM stations already matched to PLK stations in the same town. They break
    ties by proximity.
    """
    if not candidates:
        return None, "no-candidate"
    unique = collapse_duplicates(candidates)
    if unique is not None:
        return unique, "unique"
    same_city = _by_city(station, candidates)
    if same_city:
        unique = collapse_duplicates(same_city)
        if unique is not None:
            return unique, "city"
    if anchors:
        scored = sorted(
            (min(_distance(c, a) for a in anchors), c.osm_id, c) for c in candidates
        )
        best_d, _, best = scored[0]
        runner_d = scored[1][0] if len(scored) > 1 else math.inf
        if best_d <= TOWN_RADIUS_KM and runner_d - best_d >= TOWN_MARGIN_KM:
            return best, "nearest"
    return None, "ambiguous"


def match_stations(
    plk: Sequence[PlkStation], osm: Sequence[OsmStation]
) -> tuple[list[Match], list[tuple[PlkStation, str]]]:
    """Match PLK stations to OSM stations by name.

    Pass 1 takes the first index tier (see ``build_index``) that has candidates and accepts the
    match if it is unambiguous (one place or same city). Pass 2 retries the ambiguous ones, using
    the pass-1 matches from the same town as anchors.
    """
    tiers = build_index(osm)
    matched: dict[int, Match] = {}
    pending: list[tuple[PlkStation, list[OsmStation], str]] = []
    unmatched: list[tuple[PlkStation, str]] = []

    for st in plk:
        if is_placeholder(st.name):
            unmatched.append((st, "placeholder"))
            continue
        if st.external_id in FOREIGN_HOMONYMS:
            unmatched.append((st, f"abroad: {FOREIGN_HOMONYMS[st.external_id]}"))
            continue
        found = False
        for kind, index in tiers:
            candidates = index.get(_plk_key(kind, st.name), [])
            if not candidates:
                continue
            chosen, how = resolve_candidates(st, candidates)
            if chosen is not None:
                matched[st.external_id] = Match(st, chosen, f"{kind}:{how}")
            else:
                pending.append((st, list(candidates), kind))
            found = True
            break
        if not found:
            unmatched.append((st, "no-candidate"))

    anchors_by_town: dict[str, list[OsmStation]] = defaultdict(list)
    for m in matched.values():
        town = town_key(m.station)
        if town:
            anchors_by_town[town].append(m.osm)

    for st, candidates, kind in pending:
        town = town_key(st)
        anchors = anchors_by_town.get(town, []) if town else []
        chosen, how = resolve_candidates(st, candidates, anchors)
        if chosen is not None:
            matched[st.external_id] = Match(st, chosen, f"{kind}:{how}")
        else:
            unmatched.append((st, f"{how} ({len(candidates)} candidates)"))

    ordered = [matched[st.external_id] for st in plk if st.external_id in matched]
    return ordered, unmatched


# --------------------------------------------------------------------------- filling gaps


def _distance_to(candidate: OsmStation, points: Sequence[tuple[float, float]]) -> float:
    return min(
        (haversine_km(candidate.latitude, candidate.longitude, lat, lon) for lat, lon in points),
        default=math.inf,
    )


def resolve_by_route(
    candidates: Sequence[OsmStation], neighbours: Sequence[tuple[float, float]]
) -> OsmStation | None:
    """Pick the one candidate the timetable places next to ``neighbours``, or ``None``.

    Candidates within ``DUPLICATE_RADIUS_KM`` of the winner are the same physical station. Any other
    candidate must be at least ``NEIGHBOUR_MARGIN_KM`` farther from the neighbours.
    """
    scored = sorted(candidates, key=lambda c: (_distance_to(c, neighbours), c.osm_id))
    if not scored or _distance_to(scored[0], neighbours) > NEIGHBOUR_RADIUS_KM:
        return None
    best = scored[0]
    margin = _distance_to(best, neighbours) + NEIGHBOUR_MARGIN_KM
    for other in scored[1:]:
        if _distance(best, other) > DUPLICATE_RADIUS_KM and _distance_to(other, neighbours) < margin:
            return None
    return best


def _qualified_candidates(station: PlkStation, osm: Iterable[OsmStation]) -> list[OsmStation]:
    """OSM stations named like ``station`` plus one regional adjective."""
    wanted = normalise_name(station.name).split()
    found: list[OsmStation] = []
    for candidate in osm:
        for name in candidate.names:
            tokens = normalise_name(name).split()
            if (
                tokens[:-1] == wanted
                and _REGIONAL_QUALIFIER.search(tokens[-1])
                and candidate not in found
            ):
                found.append(candidate)
    return found


def _without_operator_suffix(name: str) -> str | None:
    tokens = normalise_name(name).split()
    kept = [tok for tok in tokens if tok not in _OPERATOR_SUFFIXES]
    return " ".join(kept) if kept and len(kept) < len(tokens) else None


def fill_missing_coordinates(
    plk: Sequence[PlkStation],
    osm: Sequence[OsmStation],
    curated: Sequence[dict[str, Any]],
    adjacent: Iterable[tuple[int, int]],
) -> tuple[list[Match], list[tuple[PlkStation, str]]]:
    """Match only the stations missing from ``curated``, preferring no match over a wrong one.

    ``match_stations`` runs first. Its result is then checked against the curated coordinates of the
    stops next to each station in the timetable:

    * a match farther than ``NEIGHBOUR_RADIUS_KM`` from every such stop is rejected;
    * a same-name tie is resolved only by ``resolve_by_route``;
    * an operator suffix may be dropped if the remaining name is one place;
    * a regional adjective may be added only if the timetable confirms it. Without neighbours a
      bare name is too weak: OSM "Stobno" near Piła is not PLK "Stobno Szczecińskie".
    """
    known = {
        int(e["external_id"]): (float(e["latitude"]), float(e["longitude"]))
        for e in curated
        if e.get("latitude") is not None and e.get("longitude") is not None
    }
    neighbours: dict[int, set[int]] = defaultdict(set)
    for a, b in adjacent:
        neighbours[a].add(b)
        neighbours[b].add(a)
    missing = [st for st in plk if st.external_id not in known]
    tiers = dict(build_index(osm))

    def points(st: PlkStation) -> list[tuple[float, float]]:
        return [known[n] for n in sorted(neighbours[st.external_id]) if n in known]

    def confirmed(st: PlkStation, candidate: OsmStation) -> bool:
        near = points(st)
        return not near or _distance_to(candidate, near) <= NEIGHBOUR_RADIUS_KM

    base_matches, base_unmatched = match_stations(missing, osm)
    matches: list[Match] = []
    unmatched: list[tuple[PlkStation, str]] = []
    for match in base_matches:
        if confirmed(match.station, match.osm):
            matches.append(match)
        else:
            unmatched.append((match.station, f"far from route neighbours ({match.osm.osm_id})"))

    for st, why in base_unmatched:
        chosen: OsmStation | None = None
        method = ""
        if why == "no-candidate":
            bare = _without_operator_suffix(st.name)
            unique = collapse_duplicates(tiers["name"].get(bare, [])) if bare else None
            if unique is not None and confirmed(st, unique):
                chosen, method = unique, "operator-suffix"
            else:
                chosen = resolve_by_route(_qualified_candidates(st, osm), points(st))
                method = "qualifier:route"
        elif why.startswith("ambiguous"):
            kind, candidates = next(
                (k, index[_plk_key(k, st.name)])
                for k, index in tiers.items()
                if _plk_key(k, st.name) in index
            )
            chosen, method = resolve_by_route(candidates, points(st)), f"{kind}:route"
        if chosen is not None:
            matches.append(Match(st, chosen, method))
        else:
            unmatched.append((st, why))
    return matches, unmatched


# --------------------------------------------------------------------------- I/O


def load_plk_stations(path: Path) -> list[PlkStation]:
    if path.suffix.lower() == ".json":
        raw = json.loads(path.read_text(encoding="utf-8"))
        rows: list[dict[str, Any]] = raw.get("stations", raw) if isinstance(raw, dict) else raw
    else:
        with path.open(encoding="utf-8", newline="") as fh:
            rows = list(csv.DictReader(fh))
    stations: list[PlkStation] = []
    for row in rows:
        if row.get("external_id") in (None, "") or not row.get("name"):
            continue
        city = row.get("city") or None
        stations.append(PlkStation(int(row["external_id"]), str(row["name"]).strip(), city))
    return stations


def load_adjacent_stops(path: Path) -> list[tuple[int, int]]:
    with path.open(encoding="utf-8", newline="") as fh:
        return [(int(row["station_id"]), int(row["next_station_id"])) for row in csv.DictReader(fh)]


def load_curated(path: Path) -> list[dict[str, Any]]:
    if not path.exists():
        return []
    payload: dict[str, Any] = json.loads(path.read_text(encoding="utf-8"))
    stations = payload.get("stations", [])
    return stations if isinstance(stations, list) else []


def build_payload(curated: Sequence[dict[str, Any]], matches: Sequence[Match]) -> dict[str, Any]:
    """Merge curated entries (kept verbatim, first) with new OSM matches (by external_id)."""
    seen = {int(e["external_id"]) for e in curated}
    generated = [
        {
            "external_id": m.station.external_id,
            "name": m.station.name,
            "latitude": round(m.osm.latitude, 5),
            "longitude": round(m.osm.longitude, 5),
        }
        for m in sorted(matches, key=lambda m: m.station.external_id)
        if m.station.external_id not in seen
    ]
    return {"source": SOURCE, "stations": [*curated, *generated]}


def _decimals(value: float) -> int:
    return len(str(value).partition(".")[2])


def is_low_precision(entry: dict[str, Any]) -> bool:
    """Return True if a curated entry has a short value and neither has full OSM precision."""
    decimals = _decimals(entry["latitude"]), _decimals(entry["longitude"])
    return min(decimals) <= LOW_PRECISION_DECIMALS and max(decimals) < GENERATED_DECIMALS


def refine_low_precision(
    curated: Sequence[dict[str, Any]], matches: Sequence[Match]
) -> tuple[list[dict[str, Any]], list[dict[str, Any]], list[tuple[dict[str, Any], str]]]:
    """Replace the coordinates of low-precision curated entries with their OSM match.

    Returns ``(entries, refined, kept)``. ``entries`` holds all curated entries in their original
    order, ``refined`` the entries that changed, and ``kept`` the low-precision entries left as
    they are, with the reason.
    """
    by_id = {m.station.external_id: m for m in matches}
    entries: list[dict[str, Any]] = []
    refined: list[dict[str, Any]] = []
    kept: list[tuple[dict[str, Any], str]] = []
    for entry in curated:
        if not is_low_precision(entry):
            entries.append(entry)
            continue
        match = by_id.get(int(entry["external_id"]))
        if match is None:
            entries.append(entry)
            kept.append((entry, "no-osm-match"))
            continue
        shift = haversine_km(
            entry["latitude"], entry["longitude"], match.osm.latitude, match.osm.longitude
        )
        if shift > MAX_REFINE_SHIFT_KM:
            entries.append(entry)
            kept.append((entry, f"too-far ({shift:.1f} km from {match.osm.osm_id})"))
            continue
        updated = {
            **entry,
            "latitude": round(match.osm.latitude, 5),
            "longitude": round(match.osm.longitude, 5),
        }
        entries.append(updated)
        refined.append(updated)
    return entries, refined, kept


def _pct(part: int, whole: int) -> str:
    return f"{part}/{whole} ({100.0 * part / whole:.1f}%)" if whole else f"{part}/0"


def print_report(
    plk: Sequence[PlkStation],
    payload: dict[str, Any],
    curated_count: int,
    unmatched: Sequence[tuple[PlkStation, str]],
    active_ids: set[int] | None,
) -> None:
    covered = {int(e["external_id"]) for e in payload["stations"]}
    plk_ids = {s.external_id for s in plk}
    print("== Station coordinate coverage ==")
    print(f"PLK stations:            {len(plk_ids)}")
    print(f"hand-curated (kept):     {curated_count}")
    print(f"added from OSM:          {len(payload['stations']) - curated_count}")
    print(f"covered (all stations):  {_pct(len(covered & plk_ids), len(plk_ids))}")
    if active_ids is not None:
        print(f"covered (active ids):    {_pct(len(covered & active_ids), len(active_ids))}")
    still_missing = [(s, why) for s, why in unmatched if s.external_id not in covered]
    print(f"\n== Unmatched stations ({len(still_missing)}) ==")
    for st, why in sorted(still_missing, key=lambda x: x[0].name):
        active = "" if active_ids is None else (" [active]" if st.external_id in active_ids else "")
        print(f"{st.external_id}\t{st.name}\t{why}{active}")


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0] if __doc__ else None)
    parser.add_argument("stations", type=Path, help="PLK stations export (CSV or JSON)")
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    parser.add_argument(
        "--curated", type=Path, default=None, help="curated entries to keep (default: --output)"
    )
    parser.add_argument("--overpass-url", default=DEFAULT_OVERPASS_URL)
    parser.add_argument(
        "--overpass-cache",
        type=Path,
        default=None,
        help="read the Overpass JSON from this file if present, otherwise fetch and save it here",
    )
    parser.add_argument(
        "--adjacent-stops",
        type=Path,
        default=None,
        help="CSV station_id,next_station_id from the timetable: only fill stations with no entry",
    )
    parser.add_argument(
        "--active-ids", type=Path, default=None, help="file of station ids (one per line) to report"
    )
    args = parser.parse_args(argv)

    plk = load_plk_stations(args.stations)
    curated = load_curated(args.curated or args.output)

    cache: Path | None = args.overpass_cache
    if cache is not None and cache.exists():
        overpass = json.loads(cache.read_text(encoding="utf-8"))
    else:
        overpass = fetch_overpass(args.overpass_url)
        if cache is not None:
            cache.write_text(json.dumps(overpass), encoding="utf-8")
    osm = parse_overpass(overpass)
    print(f"OSM stations/halts with a name: {len(osm)}", file=sys.stderr)

    matches, unmatched = match_stations(plk, osm)
    curated, refined, kept = refine_low_precision(curated, matches)
    print(f"low-precision entries refined from OSM: {len(refined)}", file=sys.stderr)
    for entry, why in kept:
        print(f"low-precision kept: {entry['external_id']}\t{entry['name']}\t{why}", file=sys.stderr)
    if args.adjacent_stops is not None:
        adjacent = load_adjacent_stops(args.adjacent_stops)
        matches, unmatched = fill_missing_coordinates(plk, osm, curated, adjacent)
    payload = build_payload(curated, matches)
    args.output.write_text(
        json.dumps(payload, ensure_ascii=False, indent=4) + "\n", encoding="utf-8"
    )

    active_ids: set[int] | None = None
    if args.active_ids is not None:
        text = args.active_ids.read_text(encoding="utf-8")
        active_ids = {int(line) for line in text.split() if line.strip()}
    print_report(plk, payload, len(curated), unmatched, active_ids)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
