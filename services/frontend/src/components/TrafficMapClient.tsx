'use client'

import 'leaflet/dist/leaflet.css'
import L from 'leaflet'
import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CircleMarker, GeoJSON, MapContainer, Pane, TileLayer, Tooltip, useMap } from 'react-leaflet'
import { gateway, type MapStation } from '@/lib/api'

// Padded bounding box around Poland — used both to fit the initial view and to
// lock panning so users can never scroll out into an empty void.
const POLAND_VIEW_BOUNDS: L.LatLngBoundsExpression = [
    [48.85, 13.85],
    [54.98, 24.4],
]
const MAX_BOUNDS = L.latLngBounds(POLAND_VIEW_BOUNDS).pad(0.15)

const POLAND_FILL_STYLE: L.PathOptions = {
    fillColor: '#10141f',
    fillOpacity: 1,
    color: '#334155',
    weight: 1.2,
}

// Everything outside Poland is painted over the railway tiles in the map background
// colour (see `.leaflet-container` in globals.css), so tracks stop exactly at the border.
const OUTSIDE_POLAND_STYLE: L.PathOptions = {
    fillColor: '#1a1d27',
    fillOpacity: 1,
    color: '#334155',
    weight: 1.2,
}

type PolandFeature = GeoJSON.Feature<GeoJSON.MultiPolygon>

// World ring with every Polish land part (mainland + Baltic islands) cut out as a hole.
// Latitude is capped at ±85 (the Web Mercator limit).
function buildOutsideMask(poland: PolandFeature): GeoJSON.Feature<GeoJSON.Polygon> {
    const worldRing: GeoJSON.Position[] = [
        [-180, -85],
        [180, -85],
        [180, 85],
        [-180, 85],
        [-180, -85],
    ]
    const holes = poland.geometry.coordinates.map((polygon) => polygon[0])
    return {
        type: 'Feature',
        properties: {},
        geometry: { type: 'Polygon', coordinates: [worldRing, ...holes] },
    }
}

const STATION_PATH_OPTIONS: L.PathOptions = {
    color: '#38bdf8',
    weight: 1.5,
    fillColor: '#0ea5e9',
    fillOpacity: 0.85,
}

// Real-world track geometry from OpenRailwayMap (OSM data, CC-BY-SA). The tiles are
// transparent, so they overlay the dark Poland fill without needing a base map. Dimmed
// so the network reads as context and the station dots stay the focal layer.
const RAILWAY_TILES_URL = 'https://{s}.tiles.openrailwaymap.org/standard/{z}/{x}/{y}.png'
const RAILWAY_TILES_ATTRIBUTION =
    'Data &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors, ' +
    'style: <a href="https://www.openrailwaymap.org/">OpenRailwayMap</a> (CC-BY-SA)'

const SELECTED_STATION_PATH_OPTIONS: L.PathOptions = {
    color: '#fde68a',
    weight: 2.5,
    fillColor: '#f59e0b',
    fillOpacity: 1,
}

// Pans (without changing zoom) to the selected station whenever the selection changes.
function PanToStation({ station }: { station?: MapStation }) {
    const map = useMap()
    useEffect(() => {
        if (station) map.panTo([station.latitude, station.longitude], { animate: true })
    }, [map, station])
    return null
}

interface TrafficMapClientProps {
    selectedStationId?: number
    onStationSelect?: (station: MapStation) => void
}

export function TrafficMapClient({ selectedStationId, onStationSelect }: TrafficMapClientProps = {}) {
    const [polandGeo, setPolandGeo] = useState<PolandFeature | null>(null)

    const { data: stationsData } = useQuery({
        queryKey: ['mapStations'],
        queryFn: gateway.getMapStations,
        staleTime: 60 * 60 * 1000,
    })

    useEffect(() => {
        fetch('/poland-border.geojson')
            .then((r) => r.json())
            .then((geojson: PolandFeature) => setPolandGeo(geojson))
            .catch(() => {
                // border shape not critical to interactivity — fail silently
            })
    }, [])

    const outsideMask = useMemo(() => (polandGeo ? buildOutsideMask(polandGeo) : null), [polandGeo])
    const selectedStation = stationsData?.stations.find((s) => s.external_id === selectedStationId)

    return (
        <div className="relative h-full w-full">
            <MapContainer
                bounds={POLAND_VIEW_BOUNDS}
                maxBounds={MAX_BOUNDS}
                maxBoundsViscosity={1.0}
                minZoom={6}
                maxZoom={19}
                worldCopyJump={false}
                style={{ width: '100%', height: '100%' }}
                zoomControl={true}
            >
                {polandGeo && (
                    <GeoJSON
                        key="poland-shape"
                        data={polandGeo}
                        style={POLAND_FILL_STYLE}
                        interactive={false}
                    />
                )}

                {/* Explicit panes pin the paint order: Poland fill (overlayPane, z 400) →
                    railway tiles → outside-Poland mask → station dots. */}
                <Pane name="railwayPane" style={{ zIndex: 410 }}>
                    <TileLayer
                        url={RAILWAY_TILES_URL}
                        attribution={RAILWAY_TILES_ATTRIBUTION}
                        subdomains={['a', 'b', 'c']}
                        maxZoom={19}
                        opacity={0.6}
                    />
                </Pane>

                <Pane name="outsideMaskPane" style={{ zIndex: 415 }}>
                    {outsideMask && (
                        <GeoJSON
                            key="outside-poland-mask"
                            data={outsideMask}
                            style={OUTSIDE_POLAND_STYLE}
                            interactive={false}
                        />
                    )}
                </Pane>

                <Pane name="stationsPane" style={{ zIndex: 420 }}>
                    {stationsData?.stations.map((station) => (
                        <CircleMarker
                            key={station.external_id}
                            center={[station.latitude, station.longitude]}
                            radius={station.external_id === selectedStationId ? 10 : 6}
                            pathOptions={station.external_id === selectedStationId ? SELECTED_STATION_PATH_OPTIONS : STATION_PATH_OPTIONS}
                            eventHandlers={{
                                click: () => onStationSelect?.(station),
                            }}
                        >
                            <Tooltip
                                direction="top"
                                offset={[0, -6]}
                                opacity={1}
                                permanent
                                className="station-label-tooltip"
                            >
                                <span className="font-semibold">{station.name}</span>
                            </Tooltip>
                        </CircleMarker>
                    ))}
                </Pane>
                <PanToStation station={selectedStation} />
            </MapContainer>
        </div>
    )
}
