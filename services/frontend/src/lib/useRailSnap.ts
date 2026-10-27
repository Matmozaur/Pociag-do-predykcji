'use client'

import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useMap, useMapEvents } from 'react-leaflet'
import type { TrainMapPoint } from '@/lib/api'
import {
    bearingDeg,
    buildRailIndex,
    pointAlongRoute,
    routeBetween,
    snapToRail,
    spreadOverlaps,
    type RailIndex,
    type RailRoute,
} from '@/lib/railSnap'

// How far (px) sliding along the route polyline may stray from the straight along-track
// shift that spreadOverlaps computed before the straight shift is used instead.
const ROUTE_SPREAD_TOLERANCE_PX = 1.5

/** Where a route-following train sits on its route, so overlap spreading can slide it along. */
export type RouteTrack = { route: RailRoute; distance: number }

/**
 * Railway line index built from the static public/poland-rail-lines.geojson. Loaded once
 * per session; null while loading or if the file can't be fetched (the map then shows
 * unsnapped positions).
 */
export function useRailIndex(enabled: boolean): RailIndex | null {
    // The built index (not just the raw JSON) is what's cached, so returning to the map
    // doesn't rebuild it or drop its route cache. It's mutable, so no structural sharing.
    const { data } = useQuery({
        queryKey: ['railLines'],
        queryFn: () =>
            fetch('/poland-rail-lines.geojson')
                .then((r) => {
                    if (!r.ok) throw new Error(`rail lines: HTTP ${r.status}`)
                    return r.json() as Promise<GeoJSON.FeatureCollection>
                })
                .then(buildRailIndex),
        staleTime: Infinity,
        gcTime: Infinity,
        structuralSharing: false,
        retry: 1,
        enabled,
    })
    return data ?? null
}

/**
 * Puts a train on the railway. With `fraction` (wall-clock share of the previous stop →
 * next stop run, en-route trains only) the train is placed that far along the railway
 * route between the two stops. Otherwise — or when the stops can't be routed — the
 * estimated `position` moves onto the nearest railway line, preferring lines that run
 * along the previous stop → next stop heading. Returns the input unchanged when there is
 * no index or no line within range.
 */
export function snapTrainPosition(
    index: RailIndex | null,
    train: TrainMapPoint,
    position: [number, number],
    fraction: number | null,
): { position: [number, number]; bearing?: number; track?: RouteTrack } {
    if (!index) return { position }
    const prev = train.previous_stop
    const next = train.next_stop
    const stops =
        prev?.latitude != null && prev.longitude != null && next?.latitude != null && next.longitude != null
            ? ([prev.latitude, prev.longitude, next.latitude, next.longitude] as const)
            : null
    if (stops && fraction != null) {
        const route = routeBetween(index, ...stops)
        if (route) {
            const distance = fraction * route.length
            const at = pointAlongRoute(route, distance)
            return { position: [at.latitude, at.longitude], bearing: at.bearing, track: { route, distance } }
        }
    }
    const heading = stops ? bearingDeg(...stops) : undefined
    const snapped = snapToRail(index, position[0], position[1], heading)
    if (!snapped) return { position }
    return { position: [snapped.latitude, snapped.longitude], bearing: snapped.bearing }
}

/**
 * Spread-out positions for markers that overlap on screen at the current zoom, keyed by id.
 * Only markers that moved are included; everything else keeps its own position. Markers
 * slide along their own track: route-following ones along the actual route polyline, other
 * snapped ones along their segment's bearing. Must be rendered inside a react-leaflet
 * MapContainer; recomputes after every zoom/pan.
 */
export function useOverlapOffsets(
    points: { id: number; latitude: number; longitude: number; bearing?: number; track?: RouteTrack }[],
): Map<number, [number, number]> {
    const map = useMap()
    // Bumped on every view change: layer-point projections depend on zoom and pan.
    // moveend also fires after every zoom, so it's the only event needed.
    const [viewVersion, setViewVersion] = useState(0)
    useMapEvents({
        moveend: () => setViewVersion((v) => v + 1),
    })

    return useMemo(() => {
        void viewVersion
        const result = new Map<number, [number, number]>()
        const bounds = map.getBounds().pad(0.2)
        const zoom = map.getZoom()
        const projected = points
            .filter((p) => bounds.contains([p.latitude, p.longitude]))
            .map((p) => {
                const { x, y } = map.latLngToLayerPoint([p.latitude, p.longitude])
                return { id: p.id, x, y, bearing: p.bearing, latitude: p.latitude, track: p.track }
            })
        const offsets = spreadOverlaps(projected, zoom)
        for (const p of projected) {
            const offset = offsets.get(p.id)
            if (!offset || (offset.dx === 0 && offset.dy === 0)) continue
            if (p.track && offset.along != null) {
                // Same shift in metres, but along the route polyline (Web Mercator scale) —
                // used only while the route stays close to the straight shift, so a curve
                // can't push the marker back onto a neighbour.
                const metersPerPx = (40_075_016.686 * Math.cos((p.latitude * Math.PI) / 180)) / (256 * 2 ** zoom)
                const distance = p.track.distance + offset.along * metersPerPx
                if (distance >= 0 && distance <= p.track.route.length) {
                    const at = pointAlongRoute(p.track.route, distance)
                    const { x, y } = map.latLngToLayerPoint([at.latitude, at.longitude])
                    if (Math.hypot(x - p.x - offset.dx, y - p.y - offset.dy) <= ROUTE_SPREAD_TOLERANCE_PX) {
                        result.set(p.id, [at.latitude, at.longitude])
                        continue
                    }
                }
            }
            const { lat, lng } = map.layerPointToLatLng([p.x + offset.dx, p.y + offset.dy])
            result.set(p.id, [lat, lng])
        }
        return result
    }, [map, points, viewVersion])
}
