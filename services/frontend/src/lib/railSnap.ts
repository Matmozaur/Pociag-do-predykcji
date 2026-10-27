// Snapping train positions onto railway line geometry, routing between stations over the
// railway network, plus screen-space spreading of markers that would otherwise sit on top
// of each other. Pure functions, no React — the hooks that wire this into the map live in
// useRailSnap.ts.
//
// The line geometry is the static public/poland-rail-lines.geojson (OSM, ODbL), built by
// scripts/build-rail-lines.mjs. Ways that meet share an identical coordinate there, so
// vertices keyed by coordinate form a routable graph.

const EARTH_RADIUS_M = 6_371_000
const DEG = Math.PI / 180
const METERS_PER_DEG_LAT = EARTH_RADIUS_M * DEG

// Grid cell size in degrees. ~5.5 km north–south, ~3.5 km east–west in Poland, so a
// 1.5 km snap radius touches at most a 2×3 block of cells (3 km: 3×4).
const CELL_DEG = 0.05
const GRID_COLS = Math.ceil(360 / CELL_DEG)

/** Furthest a train may be moved onto a line. */
export const SNAP_MAX_METERS = 1500
/**
 * Furthest a station may be moved onto a line when routing between stops. Larger than
 * SNAP_MAX_METERS because some stop coordinates come with only 2–3 decimals (≈ 1–2 km off).
 */
export const STATION_SNAP_MAX_METERS = 3000
/** Lines whose bearing is within ± this of the train's heading are preferred. */
export const SNAP_HEADING_TOLERANCE_DEG = 45
/**
 * A route between two stations may be at most max(ROUTE_MAX_DETOUR × straight distance,
 * straight distance + ROUTE_MAX_EXTRA_METERS) long; longer detours mean the stations
 * aren't really connected in the extract.
 */
export const ROUTE_MAX_DETOUR = 3
export const ROUTE_MAX_EXTRA_METERS = 20_000
/** A* gives up after settling this many vertices. */
export const ROUTE_MAX_VISITS = 40_000
/** Cached routes per index (oldest dropped first). */
const ROUTE_CACHE_SIZE = 2_000

/** A polyline along the railway with cumulative lengths, for O(log n) position lookups. */
export type RailRoute = {
    lats: Float64Array
    lngs: Float64Array
    /** Metres from the start to each vertex; cum[0] = 0. */
    cum: Float64Array
    length: number
}

export type RailIndex = {
    /** Segment endpoints, flat: [lat1, lng1, lat2, lng2] per segment. */
    segments: Float64Array
    /** Graph vertex ids of each segment's endpoints: [a, b] per segment. */
    segmentVertices: Int32Array
    /** Segment lengths in metres. */
    segmentLengths: Float64Array
    /** Grid cell key → indices of segments whose bounding box touches the cell. */
    cells: Map<number, number[]>
    /** Per-segment "last seen in query N" marks, so a lookup visits each segment once. */
    seen: Uint32Array
    query: number
    /** Graph vertices, flat: [lat, lng] per vertex. */
    vertices: Float64Array
    /** CSR adjacency: neighbours of v are adjVertex[adjStart[v] .. adjStart[v + 1]). */
    adjStart: Int32Array
    adjVertex: Int32Array
    adjLength: Float64Array
    /** Connected-component id of each vertex, and the vertex count of each component. */
    component: Int32Array
    componentSize: Int32Array
    /** A* scratch, reused across searches (stamped, so never cleared). */
    gScore: Float64Array
    cameFrom: Int32Array
    visitStamp: Uint32Array
    search: number
    /** Station pair key → route, or null when no route was found. */
    routes: Map<string, RailRoute | null>
}

function cellKey(row: number, col: number): number {
    return row * GRID_COLS + col
}

const cellRow = (lat: number) => Math.floor((lat + 90) / CELL_DEG)
const cellCol = (lng: number) => Math.floor((lng + 180) / CELL_DEG)

// Equirectangular distance in metres; accurate to well under 1% at segment/route scale.
function distanceM(lat1: number, lng1: number, lat2: number, lng2: number): number {
    const kx = METERS_PER_DEG_LAT * Math.cos(((lat1 + lat2) / 2) * DEG)
    return Math.hypot((lng2 - lng1) * kx, (lat2 - lat1) * METERS_PER_DEG_LAT)
}

/** Builds a uniform-grid spatial index and routing graph of every LineString / MultiLineString. */
export function buildRailIndex(geojson: GeoJSON.FeatureCollection): RailIndex {
    const vertexIds = new Map<string, number>()
    const vertexFlat: number[] = []
    const vertexId = ([lng, lat]: GeoJSON.Position): number => {
        const key = `${lng},${lat}`
        let id = vertexIds.get(key)
        if (id === undefined) {
            id = vertexFlat.length / 2
            vertexIds.set(key, id)
            vertexFlat.push(lat, lng)
        }
        return id
    }
    const flat: number[] = []
    const ends: number[] = []
    const addLine = (coords: GeoJSON.Position[]) => {
        for (let i = 1; i < coords.length; i++) {
            const [lng1, lat1] = coords[i - 1]
            const [lng2, lat2] = coords[i]
            if (lng1 === lng2 && lat1 === lat2) continue
            flat.push(lat1, lng1, lat2, lng2)
            ends.push(vertexId(coords[i - 1]), vertexId(coords[i]))
        }
    }
    for (const feature of geojson.features) {
        const geometry = feature.geometry
        if (geometry?.type === 'LineString') addLine(geometry.coordinates)
        else if (geometry?.type === 'MultiLineString') geometry.coordinates.forEach(addLine)
    }

    const segments = Float64Array.from(flat)
    const segmentVertices = Int32Array.from(ends)
    const count = segments.length / 4
    const segmentLengths = new Float64Array(count)
    const cells = new Map<number, number[]>()
    for (let s = 0; s < count; s++) {
        const o = s * 4
        segmentLengths[s] = distanceM(segments[o], segments[o + 1], segments[o + 2], segments[o + 3])
        const r0 = cellRow(Math.min(segments[o], segments[o + 2]))
        const r1 = cellRow(Math.max(segments[o], segments[o + 2]))
        const c0 = cellCol(Math.min(segments[o + 1], segments[o + 3]))
        const c1 = cellCol(Math.max(segments[o + 1], segments[o + 3]))
        for (let r = r0; r <= r1; r++) {
            for (let c = c0; c <= c1; c++) {
                const key = cellKey(r, c)
                const bucket = cells.get(key)
                if (bucket) bucket.push(s)
                else cells.set(key, [s])
            }
        }
    }

    // Undirected CSR adjacency, one edge per segment.
    const vertexCount = vertexFlat.length / 2
    const adjStart = new Int32Array(vertexCount + 1)
    for (let s = 0; s < count; s++) {
        adjStart[segmentVertices[2 * s] + 1]++
        adjStart[segmentVertices[2 * s + 1] + 1]++
    }
    for (let v = 0; v < vertexCount; v++) adjStart[v + 1] += adjStart[v]
    const fill = adjStart.slice(0, vertexCount)
    const adjVertex = new Int32Array(2 * count)
    const adjLength = new Float64Array(2 * count)
    for (let s = 0; s < count; s++) {
        const a = segmentVertices[2 * s]
        const b = segmentVertices[2 * s + 1]
        adjVertex[fill[a]] = b
        adjLength[fill[a]++] = segmentLengths[s]
        adjVertex[fill[b]] = a
        adjLength[fill[b]++] = segmentLengths[s]
    }

    // Connected components (iterative flood fill), so routing can tell when two stations
    // snapped onto pieces of track that don't connect in the extract.
    const component = new Int32Array(vertexCount).fill(-1)
    const sizes: number[] = []
    const stack: number[] = []
    for (let root = 0; root < vertexCount; root++) {
        if (component[root] >= 0) continue
        const id = sizes.length
        let size = 0
        component[root] = id
        stack.push(root)
        while (stack.length) {
            const v = stack.pop() as number
            size++
            for (let e = adjStart[v]; e < adjStart[v + 1]; e++) {
                const w = adjVertex[e]
                if (component[w] < 0) {
                    component[w] = id
                    stack.push(w)
                }
            }
        }
        sizes.push(size)
    }

    return {
        segments,
        segmentVertices,
        segmentLengths,
        cells,
        seen: new Uint32Array(count),
        query: 0,
        vertices: Float64Array.from(vertexFlat),
        adjStart,
        adjVertex,
        adjLength,
        component,
        componentSize: Int32Array.from(sizes),
        gScore: new Float64Array(vertexCount),
        cameFrom: new Int32Array(vertexCount),
        visitStamp: new Uint32Array(vertexCount),
        search: 0,
        routes: new Map(),
    }
}

/** Initial great-circle bearing from point 1 to point 2, in degrees [0, 360). */
export function bearingDeg(lat1: number, lng1: number, lat2: number, lng2: number): number {
    const phi1 = lat1 * DEG
    const phi2 = lat2 * DEG
    const dLambda = (lng2 - lng1) * DEG
    const y = Math.sin(dLambda) * Math.cos(phi2)
    const x = Math.cos(phi1) * Math.sin(phi2) - Math.sin(phi1) * Math.cos(phi2) * Math.cos(dLambda)
    return (Math.atan2(y, x) / DEG + 360) % 360
}

// Angle between two undirected lines, in degrees [0, 90].
function axialDiff(a: number, b: number): number {
    const d = Math.abs(a - b) % 180
    return Math.min(d, 180 - d)
}

// Nearest segment within maxMeters (preferring ones aligned with `heading`, and only in
// graph component `component` when given), as segment index + parameter t along it; null
// when nothing is in range.
function nearestSegment(
    index: RailIndex,
    lat: number,
    lng: number,
    heading: number | undefined,
    maxMeters: number,
    component?: number,
): { s: number; t: number } | null {
    const { segments, cells, seen, segmentVertices } = index
    // Local equirectangular projection centred on the query point (metres).
    const kx = METERS_PER_DEG_LAT * Math.cos(lat * DEG)
    const ky = METERS_PER_DEG_LAT
    const dLat = maxMeters / ky
    const dLng = maxMeters / kx
    const maxSq = maxMeters * maxMeters

    index.query = (index.query + 1) >>> 0 || 1
    const stamp = index.query

    let bestSq = Infinity
    let best = -1
    let bestT = 0
    let alignedSq = Infinity
    let aligned = -1
    let alignedT = 0

    for (let r = cellRow(lat - dLat); r <= cellRow(lat + dLat); r++) {
        for (let c = cellCol(lng - dLng); c <= cellCol(lng + dLng); c++) {
            const bucket = cells.get(cellKey(r, c))
            if (!bucket) continue
            for (const s of bucket) {
                if (seen[s] === stamp) continue
                seen[s] = stamp
                if (component != null && index.component[segmentVertices[2 * s]] !== component) continue
                const o = s * 4
                const ax = (segments[o + 1] - lng) * kx
                const ay = (segments[o] - lat) * ky
                const bx = (segments[o + 3] - lng) * kx
                const by = (segments[o + 2] - lat) * ky
                const ex = bx - ax
                const ey = by - ay
                const len2 = ex * ex + ey * ey
                const t = len2 === 0 ? 0 : Math.max(0, Math.min(1, -(ax * ex + ay * ey) / len2))
                const px = ax + t * ex
                const py = ay + t * ey
                const distSq = px * px + py * py
                if (distSq > maxSq) continue
                if (distSq < bestSq) {
                    bestSq = distSq
                    best = s
                    bestT = t
                }
                if (heading != null && distSq < alignedSq) {
                    const segBearing = (Math.atan2(ex, ey) / DEG + 360) % 360
                    if (axialDiff(segBearing, heading) <= SNAP_HEADING_TOLERANCE_DEG) {
                        alignedSq = distSq
                        aligned = s
                        alignedT = t
                    }
                }
            }
        }
    }

    if (aligned >= 0) return { s: aligned, t: alignedT }
    return best >= 0 ? { s: best, t: bestT } : null
}

/**
 * Projects (lat, lng) onto the nearest railway segment within SNAP_MAX_METERS.
 *
 * When `heading` (degrees, previous stop → next stop) is given, segments running within
 * ±SNAP_HEADING_TOLERANCE_DEG of it win over closer crossing/branching lines; if none
 * qualifies the nearest segment overall is used. The returned bearing is the segment's,
 * oriented to agree with `heading` when given. Returns null when no line is in range.
 */
export function snapToRail(
    index: RailIndex,
    lat: number,
    lng: number,
    heading?: number,
): { latitude: number; longitude: number; bearing: number } | null {
    const hit = nearestSegment(index, lat, lng, heading, SNAP_MAX_METERS)
    if (!hit) return null
    const o = hit.s * 4
    const { segments } = index
    const lat1 = segments[o]
    const lng1 = segments[o + 1]
    const lat2 = segments[o + 2]
    const lng2 = segments[o + 3]
    let bearing = bearingDeg(lat1, lng1, lat2, lng2)
    if (heading != null && Math.abs(((bearing - heading + 540) % 360) - 180) > 90) bearing = (bearing + 180) % 360
    return { latitude: lat1 + hit.t * (lat2 - lat1), longitude: lng1 + hit.t * (lng2 - lng1), bearing }
}

function makeRoute(points: number[]): RailRoute {
    // points: flat [lat, lng, …]; consecutive duplicates dropped.
    const lats: number[] = []
    const lngs: number[] = []
    for (let i = 0; i < points.length; i += 2) {
        const n = lats.length
        if (n > 0 && lats[n - 1] === points[i] && lngs[n - 1] === points[i + 1]) continue
        lats.push(points[i])
        lngs.push(points[i + 1])
    }
    const cum = new Float64Array(lats.length)
    for (let i = 1; i < lats.length; i++) cum[i] = cum[i - 1] + distanceM(lats[i - 1], lngs[i - 1], lats[i], lngs[i])
    return { lats: Float64Array.from(lats), lngs: Float64Array.from(lngs), cum, length: cum[cum.length - 1] ?? 0 }
}

// Minimal binary min-heap of (priority, vertex) with lazy deletion.
class MinHeap {
    private keys: number[] = []
    private values: number[] = []
    get size() {
        return this.keys.length
    }
    push(key: number, value: number) {
        const { keys, values } = this
        let i = keys.length
        keys.push(key)
        values.push(value)
        while (i > 0) {
            const parent = (i - 1) >> 1
            if (keys[parent] <= key) break
            keys[i] = keys[parent]
            values[i] = values[parent]
            i = parent
        }
        keys[i] = key
        values[i] = value
    }
    peekKey(): number {
        return this.keys[0]
    }
    pop(): number {
        const { keys, values } = this
        const top = values[0]
        const lastKey = keys.pop() as number
        const lastValue = values.pop() as number
        const n = keys.length
        if (n > 0) {
            let i = 0
            for (;;) {
                let child = 2 * i + 1
                if (child >= n) break
                if (child + 1 < n && keys[child + 1] < keys[child]) child++
                if (keys[child] >= lastKey) break
                keys[i] = keys[child]
                values[i] = values[child]
                i = child
            }
            keys[i] = lastKey
            values[i] = lastValue
        }
        return top
    }
}

// A* over the rail graph from a point on segment `from` to a point on segment `to`.
// Returns the route polyline or null when none exists within maxLength / ROUTE_MAX_VISITS.
function searchRoute(
    index: RailIndex,
    from: { s: number; t: number; lat: number; lng: number },
    to: { s: number; t: number; lat: number; lng: number },
    maxLength: number,
): RailRoute | null {
    const { vertices, adjStart, adjVertex, adjLength, gScore, cameFrom, visitStamp, segmentVertices, segmentLengths } = index
    if (from.s === to.s) return makeRoute([from.lat, from.lng, to.lat, to.lng])

    index.search = (index.search + 1) >>> 0 || 1
    const stamp = index.search
    // Longitude scale at the extract's northern edge (Poland tops out at ~54.9°N), never
    // larger than any edge's midpoint scale, so the heuristic can't overestimate.
    const kx = METERS_PER_DEG_LAT * Math.cos(Math.max(Math.abs(to.lat), 55) * DEG)
    const h = (v: number) => Math.hypot((vertices[2 * v + 1] - to.lng) * kx, (vertices[2 * v] - to.lat) * METERS_PER_DEG_LAT)

    const heap = new MinHeap()
    const seed = (v: number, g: number) => {
        if (visitStamp[v] === stamp && gScore[v] <= g) return
        visitStamp[v] = stamp
        gScore[v] = g
        cameFrom[v] = -1
        heap.push(g + h(v), v)
    }
    seed(segmentVertices[2 * from.s], from.t * segmentLengths[from.s])
    seed(segmentVertices[2 * from.s + 1], (1 - from.t) * segmentLengths[from.s])

    // Cost from each end vertex of the target segment to the target point.
    const endA = segmentVertices[2 * to.s]
    const endB = segmentVertices[2 * to.s + 1]
    const tailA = to.t * segmentLengths[to.s]
    const tailB = (1 - to.t) * segmentLengths[to.s]

    let best = maxLength
    let bestEnd = -1
    let visits = 0
    while (heap.size > 0 && heap.peekKey() < best) {
        const f = heap.peekKey()
        const v = heap.pop()
        const g = gScore[v]
        if (f > g + h(v) + 1e-6) continue // stale entry
        if (++visits > ROUTE_MAX_VISITS) break
        if (v === endA && g + tailA < best) {
            best = g + tailA
            bestEnd = v
        }
        if (v === endB && g + tailB < best) {
            best = g + tailB
            bestEnd = v
        }
        for (let e = adjStart[v]; e < adjStart[v + 1]; e++) {
            const w = adjVertex[e]
            const gw = g + adjLength[e]
            if (visitStamp[w] === stamp && gScore[w] <= gw) continue
            const fw = gw + h(w)
            if (fw >= best) continue
            visitStamp[w] = stamp
            gScore[w] = gw
            cameFrom[w] = v
            heap.push(fw, w)
        }
    }
    if (bestEnd < 0) return null

    const path: number[] = []
    for (let v = bestEnd; v >= 0; v = cameFrom[v]) path.push(vertices[2 * v], vertices[2 * v + 1])
    // path runs end → start as flat [lat, lng] pairs; reverse pairwise and add the endpoints.
    const points: number[] = [from.lat, from.lng]
    for (let i = path.length - 2; i >= 0; i -= 2) points.push(path[i], path[i + 1])
    points.push(to.lat, to.lng)
    return makeRoute(points)
}

function projectOnSegment(index: RailIndex, hit: { s: number; t: number }) {
    const o = hit.s * 4
    const { segments } = index
    return {
        ...hit,
        lat: segments[o] + hit.t * (segments[o + 2] - segments[o]),
        lng: segments[o + 1] + hit.t * (segments[o + 3] - segments[o + 1]),
    }
}

/**
 * The railway route between two stations: both are snapped onto the network (within
 * STATION_SNAP_MAX_METERS) and joined by the shortest path no longer than
 * max(ROUTE_MAX_DETOUR × straight, straight + ROUTE_MAX_EXTRA_METERS). Cached per
 * coordinate pair on the index; null when the stations can't be connected.
 */
export function routeBetween(index: RailIndex, lat1: number, lng1: number, lat2: number, lng2: number): RailRoute | null {
    const key = `${lat1},${lng1}|${lat2},${lng2}`
    const { routes } = index
    const cached = routes.get(key)
    if (cached !== undefined) return cached

    let route: RailRoute | null = null
    const heading = bearingDeg(lat1, lng1, lat2, lng2)
    let a = nearestSegment(index, lat1, lng1, heading, STATION_SNAP_MAX_METERS)
    let b = a && nearestSegment(index, lat2, lng2, heading, STATION_SNAP_MAX_METERS)
    if (a && b) {
        // Stations on disconnected pieces of track (an isolated station area): move the end in
        // the smaller piece onto the other end's piece, if one is in range.
        const { component, componentSize, segmentVertices } = index
        const ca = component[segmentVertices[2 * a.s]]
        const cb = component[segmentVertices[2 * b.s]]
        if (ca !== cb) {
            if (componentSize[ca] < componentSize[cb]) a = nearestSegment(index, lat1, lng1, heading, STATION_SNAP_MAX_METERS, cb)
            else b = nearestSegment(index, lat2, lng2, heading, STATION_SNAP_MAX_METERS, ca)
        }
    }
    if (a && b) {
        const from = projectOnSegment(index, a)
        const to = projectOnSegment(index, b)
        const straight = distanceM(from.lat, from.lng, to.lat, to.lng)
        route = searchRoute(index, from, to, Math.max(ROUTE_MAX_DETOUR * straight, straight + ROUTE_MAX_EXTRA_METERS))
    }

    if (routes.size >= ROUTE_CACHE_SIZE) routes.delete(routes.keys().next().value as string)
    routes.set(key, route)
    return route
}

/** Point `distance` metres along the route (clamped), with the local direction of travel. */
export function pointAlongRoute(
    route: RailRoute,
    distance: number,
): { latitude: number; longitude: number; bearing?: number } {
    const { lats, lngs, cum, length } = route
    if (lats.length < 2) return { latitude: lats[0], longitude: lngs[0] }
    const d = Math.min(length, Math.max(0, distance))
    // Last vertex with cum <= d, kept below the final vertex so i + 1 exists.
    let lo = 0
    let hi = lats.length - 2
    while (lo < hi) {
        const mid = (lo + hi + 1) >> 1
        if (cum[mid] <= d) lo = mid
        else hi = mid - 1
    }
    const span = cum[lo + 1] - cum[lo]
    const t = span > 0 ? (d - cum[lo]) / span : 0
    return {
        latitude: lats[lo] + t * (lats[lo + 1] - lats[lo]),
        longitude: lngs[lo] + t * (lngs[lo + 1] - lngs[lo]),
        bearing: bearingDeg(lats[lo], lngs[lo], lats[lo + 1], lngs[lo + 1]),
    }
}

// Screen-space de-overlap. Below this zoom the whole country is on screen and spreading
// markers would just smear them along the lines, so overlaps are left alone.
export const OVERLAP_MIN_ZOOM = 8
/** Target minimum gap between marker centres (px). */
export const OVERLAP_SPACING_PX = 8
/** Relaxation passes; each one resolves every overlap it finds a little. */
const OVERLAP_ITERATIONS = 120
// Below this |cos| between the push direction and a marker's track, the push is treated
// as sideways and the marker slides along its track by a deterministic (id-ordered) side.
const SIDEWAYS_COS = 0.3

/**
 * Pixel offsets that pull overlapping markers apart, keyed by id (only for markers that
 * moved). A marker with a `bearing` only ever slides along its own track (the straight line
 * through it at that bearing), so markers on different lines diverge along their own lines
 * and markers on one line string out along it; a marker without a bearing is pushed freely.
 * `along` is the signed shift in px in the direction of `bearing`. Iterative relaxation in id
 * order, so the layout is deterministic and stable between refreshes.
 */
export function spreadOverlaps<T extends { id: number; x: number; y: number; bearing?: number }>(
    points: T[],
    zoom: number,
): Map<number, { dx: number; dy: number; along?: number }> {
    const offsets = new Map<number, { dx: number; dy: number; along?: number }>()
    if (zoom < OVERLAP_MIN_ZOOM || points.length < 2) return offsets

    const sorted = [...points].sort((a, b) => a.id - b.id)
    const n = sorted.length
    const x = Float64Array.from(sorted, (p) => p.x)
    const y = Float64Array.from(sorted, (p) => p.y)
    // Track direction on screen (y grows downwards, so bearing b runs along (sin b, −cos b)).
    const ux = new Float64Array(n)
    const uy = new Float64Array(n)
    const hasAxis = new Uint8Array(n)
    const along = new Float64Array(n)
    sorted.forEach((p, i) => {
        if (p.bearing == null) return
        hasAxis[i] = 1
        ux[i] = Math.sin(p.bearing * DEG)
        uy[i] = -Math.cos(p.bearing * DEG)
    })

    const spacing = OVERLAP_SPACING_PX
    const spacingSq = spacing * spacing
    // Moves marker i by `need` px in direction (dx, dy) — along its track if it has one.
    // `side` (±1) is used when the push is (nearly) perpendicular to the track.
    const push = (i: number, dx: number, dy: number, need: number, side: number) => {
        if (!hasAxis[i]) {
            x[i] += dx * need
            y[i] += dy * need
            return
        }
        const cos = dx * ux[i] + dy * uy[i]
        const step = Math.abs(cos) >= SIDEWAYS_COS ? Math.sign(cos) * Math.min(need / Math.abs(cos), spacing) : side * need
        x[i] += step * ux[i]
        y[i] += step * uy[i]
        along[i] += step
    }

    const buckets = new Map<number, number[]>()
    for (let iter = 0; iter < OVERLAP_ITERATIONS; iter++) {
        buckets.clear()
        for (let i = 0; i < n; i++) {
            const k = Math.floor(x[i] / spacing) * 100_003 + Math.floor(y[i] / spacing)
            const bucket = buckets.get(k)
            if (bucket) bucket.push(i)
            else buckets.set(k, [i])
        }
        let moved = false
        for (let i = 0; i < n; i++) {
            const cx = Math.floor(x[i] / spacing)
            const cy = Math.floor(y[i] / spacing)
            for (let gx = cx - 1; gx <= cx + 1; gx++) {
                for (let gy = cy - 1; gy <= cy + 1; gy++) {
                    for (const j of buckets.get(gx * 100_003 + gy) ?? []) {
                        if (j <= i) continue
                        let ex = x[i] - x[j]
                        let ey = y[i] - y[j]
                        const distSq = ex * ex + ey * ey
                        if (distSq >= spacingSq) continue
                        const dist = Math.sqrt(distSq)
                        // Reference direction for ties/sideways pushes: i's track (else j's, else
                        // horizontal); i (lower id) slides back along it, j forward.
                        let rx = 1
                        let ry = 0
                        if (hasAxis[i]) {
                            rx = ux[i]
                            ry = uy[i]
                        } else if (hasAxis[j]) {
                            rx = ux[j]
                            ry = uy[j]
                        }
                        if (dist < 1e-6) {
                            ex = -rx
                            ey = -ry
                        } else {
                            ex /= dist
                            ey /= dist
                        }
                        const need = (spacing - dist) / 2 + 0.05
                        const sideJ = hasAxis[j] && rx * ux[j] + ry * uy[j] < 0 ? -1 : 1
                        push(i, ex, ey, need, -1)
                        push(j, -ex, -ey, need, sideJ)
                        moved = true
                    }
                }
            }
        }
        if (!moved) break
    }

    sorted.forEach((p, i) => {
        const dx = x[i] - p.x
        const dy = y[i] - p.y
        if (dx === 0 && dy === 0) return
        offsets.set(p.id, hasAxis[i] ? { dx, dy, along: along[i] } : { dx, dy })
    })
    return offsets
}
