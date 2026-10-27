// Generates public/poland-rail-lines.geojson — the simplified Polish mainline/branch
// railway network the /mapa page snaps train markers onto (see src/lib/railSnap.ts).
//
// Data: OpenStreetMap via the Overpass API (railway=rail without a service tag, any usage
// except industrial/military/tourism/test — station through-tracks often carry no usage
// tag at all, so filtering on usage=main|branch leaves gaps at stations), plus main/branch
// lines tagged railway=construction — lines being rebuilt, which the timetable still serves —
// and service=crossover ways: in OSM each track of a double-track line is its own way and
// the tracks only meet through crossovers, so without them the network falls apart into
// per-track pieces that can't be routed across. © OpenStreetMap contributors, licensed under the ODbL
// (https://www.openstreetmap.org/copyright) — keep the OSM attribution on the map.
//
// Rerun only when the network has changed noticeably (new/closed lines); the output is
// committed. From services/frontend/ (Node >= 18, no dependencies):
//
//     node scripts/build-rail-lines.mjs [toleranceMeters=15]
//
// Each OSM way becomes one LineString, simplified with Douglas–Peucker in a local
// equirectangular projection, coordinates rounded to 5 decimals, properties dropped.
// Way endpoints and nodes shared by two or more ways (junctions) are never simplified
// away, so connected ways meet at identical coordinates and the client can route over
// the network (src/lib/railSnap.ts buildRailIndex).
//
// Set OVERPASS_CACHE=<path> to reuse a saved Overpass response (written on first run).

import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const ENDPOINTS = [
    'https://overpass-api.de/api/interpreter',
    'https://overpass.kumi.systems/api/interpreter',
    'https://maps.mail.ru/osm/tools/overpass/api/interpreter',
]
const USER_AGENT = 'pociag-do-predykcji/rail-lines-generator (one-off build script)'
const OUT_FILE = join(dirname(fileURLToPath(import.meta.url)), '..', 'public', 'poland-rail-lines.geojson')
const TOLERANCE_M = Number(process.argv[2] ?? 15)

const QUERY = `
[out:json][timeout:300];
area["ISO3166-1"="PL"][admin_level=2]->.pl;
(
  way(area.pl)["railway"="rail"][!"service"]["usage"!~"^(industrial|military|tourism|test)$"];
  way(area.pl)["railway"="construction"]["usage"~"^(main|branch)$"][!"service"];
  way(area.pl)["railway"="rail"]["service"="crossover"];
);
out body;
>;
out skel qt;
`

const EARTH_RADIUS_M = 6_371_000
const DEG = Math.PI / 180

// The public servers answer 502/504 "at capacity" when busy and ask clients to retry with
// exponential backoff, so each endpoint gets a few attempts (10 s, 20 s, 40 s, …).
const ATTEMPTS_PER_ENDPOINT = 5

async function fetchOverpass() {
    for (const url of ENDPOINTS) {
        for (let attempt = 1; attempt <= ATTEMPTS_PER_ENDPOINT; attempt++) {
            try {
                console.log(`Querying ${url} (attempt ${attempt}) …`)
                const res = await fetch(url, {
                    method: 'POST',
                    headers: { 'User-Agent': USER_AGENT, 'Content-Type': 'application/x-www-form-urlencoded' },
                    body: 'data=' + encodeURIComponent(QUERY),
                    signal: AbortSignal.timeout(400_000),
                })
                if (!res.ok) throw new Error(`HTTP ${res.status}`)
                return await res.json()
            } catch (err) {
                console.warn(`  failed: ${err.message}`)
                if (attempt < ATTEMPTS_PER_ENDPOINT) await new Promise((r) => setTimeout(r, 10_000 * 2 ** (attempt - 1)))
            }
        }
    }
    throw new Error('All Overpass endpoints failed')
}

// Perpendicular distance (metres) of p from segment a–b, all as [x, y] metres.
function segmentDistance(p, a, b) {
    const dx = b[0] - a[0]
    const dy = b[1] - a[1]
    const len2 = dx * dx + dy * dy
    let t = len2 === 0 ? 0 : ((p[0] - a[0]) * dx + (p[1] - a[1]) * dy) / len2
    t = Math.max(0, Math.min(1, t))
    return Math.hypot(p[0] - (a[0] + t * dx), p[1] - (a[1] + t * dy))
}

// Iterative Douglas–Peucker over [lng, lat] points, using a local equirectangular
// projection centred on the way (accurate to well under a metre at way scale).
// `fixed[i]` marks points that must survive (junctions).
function simplify(coords, toleranceM, fixed) {
    if (coords.length <= 2) return coords
    const lat0 = coords.reduce((s, c) => s + c[1], 0) / coords.length
    const kx = EARTH_RADIUS_M * DEG * Math.cos(lat0 * DEG)
    const ky = EARTH_RADIUS_M * DEG
    const xy = coords.map(([lng, lat]) => [lng * kx, lat * ky])

    const keep = new Uint8Array(coords.length)
    keep[0] = keep[coords.length - 1] = 1
    // Split at fixed points first so every run between them is simplified on its own.
    const stack = []
    let start = 0
    for (let i = 1; i < coords.length; i++) {
        if (i === coords.length - 1 || fixed[i]) {
            keep[i] = 1
            stack.push([start, i])
            start = i
        }
    }
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

const round5 = (v) => Math.round(v * 1e5) / 1e5

const CACHE = process.env.OVERPASS_CACHE
let data
if (CACHE && existsSync(CACHE)) {
    data = JSON.parse(readFileSync(CACHE, 'utf8'))
} else {
    data = await fetchOverpass()
    if (CACHE) writeFileSync(CACHE, JSON.stringify(data))
}

const nodes = new Map()
const ways = []
for (const el of data.elements ?? []) {
    if (el.type === 'node') nodes.set(el.id, [el.lon, el.lat])
    else if (el.type === 'way' && Array.isArray(el.nodes) && el.nodes.length >= 2) ways.push(el.nodes)
}
// How many ways reference each node (a way that loops through a node counts it twice).
const uses = new Map()
for (const ids of ways) for (const id of ids) uses.set(id, (uses.get(id) ?? 0) + 1)

const features = []
let vertices = 0
for (const ids of ways) {
    const raw = []
    const fixed = []
    for (const id of ids) {
        const c = nodes.get(id)
        if (!c) continue
        raw.push(c)
        fixed.push((uses.get(id) ?? 0) >= 2)
    }
    if (raw.length < 2) continue
    const simplified = simplify(raw, TOLERANCE_M, fixed).map(([lng, lat]) => [round5(lng), round5(lat)])
    // Rounding can collapse consecutive points; drop the duplicates.
    const coords = simplified.filter((c, i) => i === 0 || c[0] !== simplified[i - 1][0] || c[1] !== simplified[i - 1][1])
    if (coords.length < 2) continue
    vertices += coords.length
    features.push({ type: 'Feature', properties: {}, geometry: { type: 'LineString', coordinates: coords } })
}

if (features.length === 0) throw new Error('Overpass returned no railway ways — refusing to write an empty file')

// The file is an OSM-derived database served publicly, so it carries its ODbL notice.
const json = JSON.stringify({
    type: 'FeatureCollection',
    attribution: '© OpenStreetMap contributors',
    license: 'ODbL-1.0',
    features,
})
writeFileSync(OUT_FILE, json)
console.log(`Tolerance: ${TOLERANCE_M} m`)
console.log(`Features: ${features.length}`)
console.log(`Vertices: ${vertices}`)
console.log(`Size: ${(Buffer.byteLength(json) / 1024 / 1024).toFixed(2)} MB → ${OUT_FILE}`)
