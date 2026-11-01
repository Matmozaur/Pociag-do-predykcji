'use client'

import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import type L from 'leaflet'
import { api, type MapStation } from '@/lib/api'
import { routeBetween, type RailIndex } from '@/lib/railSnap'

/**
 * Whole route of one train as Leaflet polylines: its non-cancelled stops in sequence order,
 * joined along the railway (routeBetween) where the rail index can route them and by a
 * straight segment otherwise. Null while there is no train, its detail isn't loaded, or
 * fewer than two of its stops have coordinates. Shares the 'trainDetail' query cache with
 * the train detail page.
 */
export function useTrainRoute(
    operationId: number | undefined,
    stations: MapStation[] | undefined,
    railIndex: RailIndex | null,
): L.LatLngTuple[][] | null {
    const { data: detail } = useQuery({
        queryKey: ['trainDetail', operationId],
        queryFn: () => api.getTrainDetail(operationId!),
        enabled: operationId != null,
        staleTime: 5 * 60_000,
    })

    const stationsById = useMemo(() => {
        const byId = new Map<number, MapStation>()
        for (const s of stations ?? []) byId.set(s.external_id, s)
        return byId
    }, [stations])

    const stopPoints = useMemo(() => {
        if (operationId == null || !detail || detail.operation_id !== operationId) return null
        const points: L.LatLngTuple[] = []
        for (const stop of [...detail.stops].sort((a, b) => a.sequence - b.sequence)) {
            if (stop.is_cancelled || stop.station_external_id == null) continue
            const station = stationsById.get(stop.station_external_id)
            if (!station) continue
            const last = points[points.length - 1]
            if (last && last[0] === station.latitude && last[1] === station.longitude) continue
            points.push([station.latitude, station.longitude])
        }
        return points.length >= 2 ? points : null
    }, [operationId, detail, stationsById])

    return useMemo(() => {
        if (!stopPoints) return null
        const line: L.LatLngTuple[] = [stopPoints[0]]
        const push = (lat: number, lng: number) => {
            const last = line[line.length - 1]
            if (last[0] !== lat || last[1] !== lng) line.push([lat, lng])
        }
        for (let i = 1; i < stopPoints.length; i++) {
            const [lat1, lng1] = stopPoints[i - 1]
            const [lat2, lng2] = stopPoints[i]
            const route = railIndex ? routeBetween(railIndex, lat1, lng1, lat2, lng2) : null
            if (route) {
                for (let j = 0; j < route.lats.length; j++) push(route.lats[j], route.lngs[j])
            }
            push(lat2, lng2)
        }
        return [line]
    }, [stopPoints, railIndex])
}
