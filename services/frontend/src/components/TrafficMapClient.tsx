'use client'

import 'leaflet/dist/leaflet.css'
import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { CircleMarker, GeoJSON, MapContainer, TileLayer, Tooltip } from 'react-leaflet'
import { gateway } from '@/lib/api'

const INITIAL_MAP_BOUNDS: [[number, number], [number, number]] = [
    [48.85, 13.85],
    [54.98, 24.4],
]

function buildMask(polandCoords: [number, number][][]): GeoJSON.Feature {
    // Outer ring: whole world (clockwise in GeoJSON = exterior for inverted polygon)
    // Inner ring: Poland border (counterclockwise = hole)
    // GeoJSON uses [longitude, latitude]
    const worldRing: [number, number][] = [
        [-180, -90],
        [180, -90],
        [180, 90],
        [-180, 90],
        [-180, -90],
    ]
    // Deep-copy and reverse to make it a hole (clockwise)
    const polandHole: [number, number][] = [...polandCoords[0]].reverse()
    return {
        type: 'Feature',
        properties: {},
        geometry: {
            type: 'Polygon',
            coordinates: [worldRing, polandHole],
        },
    }
}

export function TrafficMapClient() {
    const [maskFeature, setMaskFeature] = useState<GeoJSON.Feature | null>(null)

    const { data: stationsData } = useQuery({
        queryKey: ['mapStations'],
        queryFn: gateway.getMapStations,
        staleTime: 60 * 60 * 1000,
    })

    useEffect(() => {
        fetch('/poland-border.geojson')
            .then((r) => r.json())
            .then((geojson: GeoJSON.Feature<GeoJSON.Polygon>) => {
                setMaskFeature(buildMask(geojson.geometry.coordinates as [number, number][][]))
            })
            .catch(() => {
                // mask not critical — fail silently
            })
    }, [])

    return (
        <div className="relative h-full w-full">
            <MapContainer
                bounds={INITIAL_MAP_BOUNDS}
                minZoom={2}
                maxZoom={19}
                worldCopyJump={false}
                style={{ width: '100%', height: '100%' }}
                zoomControl={true}
            >
                <TileLayer
                    url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png"
                    attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> &copy; <a href="https://carto.com">CARTO</a>'
                    subdomains="abcd"
                    maxZoom={20}
                    noWrap={true}
                    keepBuffer={2}
                />

                <TileLayer
                    url="https://{s}.tiles.openrailwaymap.org/standard/{z}/{x}/{y}.png"
                    attribution='Map style: &copy; <a href="https://www.openrailwaymap.org/">OpenRailwayMap</a> (CC-BY-SA)'
                    subdomains="abc"
                    opacity={0.55}
                    zIndex={200}
                    maxZoom={19}
                    noWrap={true}
                    keepBuffer={2}
                    updateWhenIdle={true}
                />

                {/* Keep the surrounding map legible but visually recess it behind Poland. */}
                {maskFeature && (
                    <GeoJSON
                        key="poland-mask"
                        data={maskFeature as GeoJSON.Feature<GeoJSON.Geometry>}
                        style={() => ({
                            fillColor: '#0a0c14',
                            fillOpacity: 0.68,
                            color: '#1e293b',
                            weight: 0.5,
                        })}
                        interactive={false}
                    />
                )}

                {stationsData?.stations.map((station) => (
                    <CircleMarker
                        key={station.external_id}
                        center={[station.latitude, station.longitude]}
                        radius={4}
                        pathOptions={{
                            color: '#38bdf8',
                            weight: 1.5,
                            fillColor: '#0ea5e9',
                            fillOpacity: 0.85,
                        }}
                    >
                        <Tooltip direction="top" offset={[0, -4]} opacity={1}>
                            <span className="font-semibold">{station.name}</span>
                            {station.city ? <span className="text-slate-400"> · {station.city}</span> : null}
                        </Tooltip>
                    </CircleMarker>
                ))}

            </MapContainer>
        </div>
    )
}
