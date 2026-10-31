// Generates public/geo-background.geojson — the rivers, lakes and city names the /mapa
// page draws under the railway network so the dark Poland fill has geographic context
// (see GeoBackground in src/components/TrafficMapClient.tsx).
//
// Data: Natural Earth 1:10m (https://www.naturalearthdata.com), public domain — no
// attribution required. Read from the nvkelso/natural-earth-vector GitHub mirror, pinned
// to commit ca96624a so reruns are reproducible (bump SOURCE deliberately to refresh):
// rivers_lake_centerlines (the big rivers: Wisła, Odra, Warta, Bug), rivers_europe (the
// regional supplement: San, Narew, Pilica, Noteć, …), lakes + lakes_europe (Zalew Wiślany,
// Zalew Szczeciński, Mazury) and populated_places (city points with Polish names).
//
// Rerun only to change the selection or styling ranks; the output is committed. From
// services/frontend/ (Node >= 18, no dependencies):
//
//     node scripts/build-geo-background.mjs [toleranceMeters=250]
//
// Rivers are clipped to BBOX (split into runs of vertices inside it) and simplified with
// Douglas–Peucker; lakes are kept whole if they touch BBOX and simplified ring by ring.
// Coordinates are rounded to 4 decimals (~10 m). Every feature keeps only `kind`
// ('river' | 'lake' | 'city'), `rank` (1 = major, 2 = minor; the client styles rivers
// and reveals city labels by it) and, for cities only, `name` — rivers and lakes are
// drawn unlabelled.
//
// Set NE_CACHE=<dir> to reuse downloaded source files (written there on first run).

import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const SOURCE = 'https://raw.githubusercontent.com/nvkelso/natural-earth-vector/ca96624a56bd078437bca8184e78163e5039ad19/geojson'
const OUT_FILE = join(dirname(fileURLToPath(import.meta.url)), '..', 'public', 'geo-background.geojson')
const TOLERANCE_M = Number(process.argv[2] ?? 250)

// Poland plus its neighbours' border regions: everything the map shows at country zoom.
// Tighter than the map's MAX_BOUNDS — the far corners are behind the outside veil anyway.
const BBOX = { west: 12, south: 48, east: 27, north: 56 }

// Natural Earth scalerank cut-offs (lower = more important).
const MAJOR_RIVER_MAX_RANK = 8 // rivers_lake_centerlines
const MINOR_RIVER_MAX_RANK = 11 // rivers_europe
const LAKE_MAX_RANK = 12 // lakes_europe

// Polish cities from populated_places: voivodeship seats and Warszawa are rank 1 (always
// labelled), other cities of at least MIN_POLISH_POP rank 2. Foreign capitals and big
// border-region cities (MIN_FOREIGN_POP) are context: rank 1 for capitals, rank 2 otherwise.
const MIN_POLISH_POP = 100_000
const MIN_FOREIGN_POP = 250_000
// Natural Earth still tags Elbląg as an Admin-1 capital (a pre-1999 voivodeship seat).
const DEMOTED_CAPITALS = new Set(['Elbląg'])
// populated_places has only ~24 Polish points, so it lacks two voivodeship seats and most
// cities over MIN_POLISH_POP (GUS 2023); they are added by hand.
const EXTRA_CITIES = [
    { name: 'Toruń', rank: 1, coordinates: [18.6046, 53.0138] },
    { name: 'Gorzów Wielkopolski', rank: 1, coordinates: [15.2288, 52.7368] },
    { name: 'Częstochowa', rank: 2, coordinates: [19.1203, 50.8118] },
    { name: 'Radom', rank: 2, coordinates: [21.1471, 51.4027] },
    { name: 'Sosnowiec', rank: 2, coordinates: [19.1041, 50.2863] },
    { name: 'Zabrze', rank: 2, coordinates: [18.7857, 50.3249] },
    { name: 'Bielsko-Biała', rank: 2, coordinates: [19.0584, 49.8224] },
    { name: 'Ruda Śląska', rank: 2, coordinates: [18.8556, 50.2558] },
    { name: 'Rybnik', rank: 2, coordinates: [18.5463, 50.1022] },
    { name: 'Tychy', rank: 2, coordinates: [18.9667, 50.1236] },
    { name: 'Dąbrowa Górnicza', rank: 2, coordinates: [19.1949, 50.3217] },
    { name: 'Płock', rank: 2, coordinates: [19.7065, 52.5463] },
    { name: 'Wałbrzych', rank: 2, coordinates: [16.2845, 50.7845] },
    { name: 'Chorzów', rank: 2, coordinates: [18.9545, 50.2975] },
    { name: 'Tarnów', rank: 2, coordinates: [20.9858, 50.0121] },
]

const EARTH_RADIUS_M = 6_371_000
const DEG = Math.PI / 180

async function load(name) {
    const cacheDir = process.env.NE_CACHE
    const cached = cacheDir && join(cacheDir, `${name}.geojson`)
    if (cached && existsSync(cached)) return JSON.parse(readFileSync(cached, 'utf8'))
    console.log(`Downloading ${name} …`)
    const res = await fetch(`${SOURCE}/${name}.geojson`, { signal: AbortSignal.timeout(120_000) })
    if (!res.ok) throw new Error(`${name}: HTTP ${res.status}`)
    const text = await res.text()
    if (cached) {
        mkdirSync(cacheDir, { recursive: true })
        writeFileSync(cached, text)
    }
    return JSON.parse(text)
}

const inBox = ([lng, lat]) => lng >= BBOX.west && lng <= BBOX.east && lat >= BBOX.south && lat <= BBOX.north

// Perpendicular distance (metres) of p from segment a–b, all as [x, y] metres.
function segmentDistance(p, a, b) {
    const dx = b[0] - a[0]
    const dy = b[1] - a[1]
    const len2 = dx * dx + dy * dy
    let t = len2 === 0 ? 0 : ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / len2
    t = Math.max(0, Math.min(1, t))
    return Math.hypot(p[0] - (a[0] + t * dx), p[1] - (a[1] + t * dy))
}

// Iterative Douglas–Peucker over [lng, lat] points in a local equirectangular projection.
function simplify(coords, toleranceM) {
    if (coords.length <= 2) return coords
    const lat0 = coords.reduce((s, c) => s + c[1], 0) / coords.length
    const kx = EARTH_RADIUS_M * DEG * Math.cos(lat0 * DEG)
    const ky = EARTH_RADIUS_M * DEG
    const xy = coords.map(([lng, lat]) => [lng * kx, lat * ky])
    const keep = new Uint8Array(coords.length)
    keep[0] = keep[coords.length - 1] = 1
    const stack = [[0, coords.length - 1]]
    while (stack.length) {
        const [first, last] = stack.pop()
        let maxDist = 0
        let index = -1
        for (let i = first + 1; i < last; i++) {
            const d = segmentDistance(xy[i], xy[first], xy[last])
            if (d > maxDist) {
                maxDist = d
                index = i
            }
        }
        if (maxDist > toleranceM) {
            keep[index] = 1
            stack.push([first, index], [index, last])
        }
    }
    return coords.filter((_, i) => keep[i])
}

const round4 = (v) => Math.round(v * 1e4) / 1e4

// Simplify, round, and drop the consecutive duplicates rounding can produce.
function clean(coords) {
    const rounded = simplify(coords, TOLERANCE_M).map(([lng, lat]) => [round4(lng), round4(lat)])
    return rounded.filter((c, i) => i === 0 || c[0] !== rounded[i - 1][0] || c[1] !== rounded[i - 1][1])
}

// Lines as arrays of [lng, lat]; MultiLineString parts are flattened.
const lineParts = (geometry) =>
    geometry.type === 'LineString' ? [geometry.coordinates] : geometry.type === 'MultiLineString' ? geometry.coordinates : []

// Splits a line into the runs that lie inside BBOX, keeping one vertex past each edge so
// rivers run off the box instead of stopping short of it.
function clipLine(coords) {
    const runs = []
    let run = []
    for (let i = 0; i < coords.length; i++) {
        const inside = inBox(coords[i])
        const nearInside = inside || (i > 0 && inBox(coords[i - 1])) || (i + 1 < coords.length && inBox(coords[i + 1]))
        if (nearInside) run.push(coords[i])
        else if (run.length) {
            runs.push(run)
            run = []
        }
    }
    if (run.length) runs.push(run)
    return runs.filter((r) => r.length >= 2)
}

const features = []

function addRivers(collection, maxRank, rank) {
    for (const f of collection.features) {
        const p = f.properties ?? {}
        if (!f.geometry || p.scalerank > maxRank) continue
        const runs = lineParts(f.geometry)
            .flatMap(clipLine)
            .map(clean)
            .filter((c) => c.length >= 2)
        if (runs.length === 0) continue
        features.push({
            type: 'Feature',
            properties: { kind: 'river', rank },
            geometry: runs.length === 1 ? { type: 'LineString', coordinates: runs[0] } : { type: 'MultiLineString', coordinates: runs },
        })
    }
}

function addLakes(collection, maxRank) {
    for (const f of collection.features) {
        const p = f.properties ?? {}
        if (!f.geometry || p.scalerank > maxRank) continue
        const polygons = f.geometry.type === 'Polygon' ? [f.geometry.coordinates] : f.geometry.type === 'MultiPolygon' ? f.geometry.coordinates : []
        const kept = polygons
            .filter((rings) => rings[0].some(inBox))
            .map((rings) => rings.map(clean).filter((ring) => ring.length >= 4))
            .filter((rings) => rings.length > 0)
        if (kept.length === 0) continue
        features.push({
            type: 'Feature',
            properties: { kind: 'lake', rank: 1 },
            geometry: kept.length === 1 ? { type: 'Polygon', coordinates: kept[0] } : { type: 'MultiPolygon', coordinates: kept },
        })
    }
}

function addCities(collection) {
    for (const f of collection.features) {
        const p = f.properties ?? {}
        const coords = f.geometry?.coordinates
        if (!coords || !inBox(coords)) continue
        const polish = p.ADM0_A3 === 'POL'
        const name = p.NAME_PL || p.NAME
        let rank
        if (polish) {
            const capital = p.FEATURECLA === 'Admin-0 capital' || p.FEATURECLA === 'Admin-1 capital'
            if (capital && !DEMOTED_CAPITALS.has(name)) rank = 1
            else if (p.POP_MAX >= MIN_POLISH_POP) rank = 2
        } else if (p.FEATURECLA === 'Admin-0 capital') rank = 1
        else if (p.POP_MAX >= MIN_FOREIGN_POP) rank = 2
        if (!rank) continue
        features.push({
            type: 'Feature',
            properties: { kind: 'city', rank, name },
            geometry: { type: 'Point', coordinates: [round4(coords[0]), round4(coords[1])] },
        })
    }
    for (const c of EXTRA_CITIES) {
        features.push({ type: 'Feature', properties: { kind: 'city', rank: c.rank, name: c.name }, geometry: { type: 'Point', coordinates: c.coordinates } })
    }
}

addLakes(await load('ne_10m_lakes'), Infinity)
addLakes(await load('ne_10m_lakes_europe'), LAKE_MAX_RANK)
addRivers(await load('ne_10m_rivers_europe'), MINOR_RIVER_MAX_RANK, 2)
addRivers(await load('ne_10m_rivers_lake_centerlines'), MAJOR_RIVER_MAX_RANK, 1)
addCities(await load('ne_10m_populated_places'))

if (features.length === 0) throw new Error('No features selected — refusing to write an empty file')

const json = JSON.stringify({ type: 'FeatureCollection', attribution: 'Natural Earth (public domain)', features })
writeFileSync(OUT_FILE, json)
const count = (kind) => features.filter((f) => f.properties.kind === kind).length
console.log(`Tolerance: ${TOLERANCE_M} m`)
console.log(`Rivers: ${count('river')}, lakes: ${count('lake')}, cities: ${count('city')}`)
console.log(`Size: ${(Buffer.byteLength(json) / 1024).toFixed(0)} KB → ${OUT_FILE}`)
