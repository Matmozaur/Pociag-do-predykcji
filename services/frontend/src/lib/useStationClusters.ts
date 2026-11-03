'use client'

import { useMemo } from 'react'
import Supercluster from 'supercluster'
import type { MapStation } from '@/lib/api'

const CLUSTER_RADIUS_PX = 30
// Past this zoom every station stands alone, so the last split happens on the way to 13.
const CLUSTER_MAX_ZOOM = 12

// The map's minZoom: lower levels are never shown, so the index doesn't build them.
const CLUSTER_MIN_ZOOM = 5
// Leaflet's tile size: Supercluster's default of 512 would make the radius half of what it says.
const CLUSTER_EXTENT = 256
const WORLD_BBOX: GeoJSON.BBox = [-180, -85, 180, 85]

type StationProps = { station: MapStation }
// No map/reduce option, so clusters carry nothing beyond Supercluster's own properties.
type StationIndex = Supercluster<StationProps, Record<never, never>>

export type StationMapItem =
    | { kind: 'station'; station: MapStation }
    | { kind: 'cluster'; id: number; latitude: number; longitude: number; count: number; expansionZoom: number }

/**
 * Stations to draw at this zoom: nearby ones merged into count bubbles, the rest as is. The
 * whole world is queried, so the result depends only on the rounded zoom and panning or a
 * fractional zoom keeps the same array.
 */
export function useStationClusters(stations: MapStation[], zoom: number): StationMapItem[] {
    const index = useMemo<StationIndex>(() => {
        const clusterIndex: StationIndex = new Supercluster({
            radius: CLUSTER_RADIUS_PX,
            extent: CLUSTER_EXTENT,
            minZoom: CLUSTER_MIN_ZOOM,
            maxZoom: CLUSTER_MAX_ZOOM,
            minPoints: 2,
        })
        return clusterIndex.load(
            stations.map((station) => ({
                type: 'Feature',
                geometry: { type: 'Point', coordinates: [station.longitude, station.latitude] },
                properties: { station },
            })),
        )
    }, [stations])

    const roundedZoom = Math.round(zoom)

    return useMemo(
        () =>
            index.getClusters(WORLD_BBOX, roundedZoom).map((feature): StationMapItem => {
                const props = feature.properties
                if ('station' in props) return { kind: 'station', station: props.station }
                const [longitude, latitude] = feature.geometry.coordinates
                return {
                    kind: 'cluster',
                    id: props.cluster_id,
                    latitude,
                    longitude,
                    count: props.point_count,
                    expansionZoom: index.getClusterExpansionZoom(props.cluster_id),
                }
            }),
        [index, roundedZoom],
    )
}
