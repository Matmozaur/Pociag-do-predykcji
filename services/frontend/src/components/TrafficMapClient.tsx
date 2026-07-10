'use client'

import 'leaflet/dist/leaflet.css'
import { useQuery } from '@tanstack/react-query'
import { GeoJSON, MapContainer, TileLayer } from 'react-leaflet'
import type { Feature, Geometry } from 'geojson'
import { circleMarker, type Layer, type PathOptions } from 'leaflet'
import { Spinner } from '@/lib/ui'
import {
    stationGeoJSON,
    type StationFeatureProperties,
    type TrafficFeatureProperties,
    type TrafficGeoJSON,
    trafficColor,
    trafficWeight,
} from '@/lib/api'

const POLAND_BOUNDS: [[number, number], [number, number]] = [
    [49.0, 14.1],
    [54.9, 24.3],
]

function MapLegend() {
    const items = [
        { color: '#3b82f6', label: 'Niski ruch (<200)' },
        { color: '#22c55e', label: 'Umiarkowany (200-400)' },
        { color: '#eab308', label: 'Duży ruch (400-600)' },
        { color: '#f97316', label: 'Bardzo duży (600-800)' },
        { color: '#ef4444', label: 'Krytyczny (>800)' },
    ]

    return (
        <div className="absolute bottom-8 right-4 z-[1000] min-w-48 rounded-xl border border-[#2d3148] bg-[#1a1d27]/95 p-3 backdrop-blur-sm">
            <p className="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-300">
                Natężenie ruchu
            </p>
            <ul className="flex flex-col gap-1.5">
                {items.map(({ color, label }) => (
                    <li key={label} className="flex items-center gap-2">
                        <span className="block h-1 w-6 flex-shrink-0 rounded-full" style={{ backgroundColor: color }} />
                        <span className="text-xs text-slate-400">{label}</span>
                    </li>
                ))}
            </ul>
            <div className="mt-2 border-t border-[#2d3148] pt-2">
                <p className="text-xs text-slate-500">Podklad: OpenRailwayMap</p>
            </div>
        </div>
    )
}

export function TrafficMapClient() {
    const { data, isLoading } = useQuery({
        queryKey: ['trafficData'],
        queryFn: async () => {
            const res = await fetch('/api/mock/traffic')
            if (!res.ok) {
                throw new Error('Błąd pobierania danych ruchu')
            }
            return res.json() as Promise<TrafficGeoJSON>
        },
        staleTime: Infinity,
    })

    return (
        <div className="relative h-full w-full">
            {isLoading && (
                <div className="pointer-events-none absolute inset-0 z-[1200] flex items-center justify-center">
                    <Spinner className="h-8 w-8" />
                </div>
            )}
            <MapContainer
                bounds={POLAND_BOUNDS}
                maxBounds={POLAND_BOUNDS}
                maxBoundsViscosity={1.0}
                minZoom={6}
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
                />

                <TileLayer
                    url="https://{s}.tiles.openrailwaymap.org/standard/{z}/{x}/{y}.png"
                    attribution='Map style: &copy; <a href="https://www.openrailwaymap.org/">OpenRailwayMap</a> (CC-BY-SA)'
                    subdomains="abcd"
                    opacity={0.55}
                    zIndex={200}
                    maxZoom={19}
                    noWrap={true}
                />

                <GeoJSON
                    key="stations"
                    data={stationGeoJSON as GeoJSON.FeatureCollection}
                    pointToLayer={(_feature, latlng) =>
                        circleMarker(latlng, {
                            radius: 4,
                            color: '#0f172a',
                            weight: 1,
                            fillColor: '#f8fafc',
                            fillOpacity: 0.95,
                        })
                    }
                    onEachFeature={(
                        feature: Feature<Geometry, StationFeatureProperties>,
                        layer: Layer,
                    ) => {
                        layer.bindPopup(
                            `<div style="font-family:system-ui;min-width:120px">
                              <p style="font-weight:600;margin:0">${feature.properties.station_name}</p>
                            </div>`,
                        )
                    }}
                />

                {data && (
                    <GeoJSON
                        key={JSON.stringify(data)}
                        data={data as GeoJSON.FeatureCollection}
                        style={(feature?: Feature<Geometry, TrafficFeatureProperties>): PathOptions => {
                            const volume = feature?.properties?.volume ?? 0
                            return {
                                color: trafficColor(volume),
                                weight: trafficWeight(volume),
                                opacity: 0.85,
                            }
                        }}
                        onEachFeature={(
                            feature: Feature<Geometry, TrafficFeatureProperties>,
                            layer: Layer,
                        ) => {
                            const { volume, line_name } = feature.properties
                            const escapedName = line_name
                                .replace(/&/g, '&amp;')
                                .replace(/</g, '&lt;')
                                .replace(/>/g, '&gt;')
                                .replace(/"/g, '&quot;')

                            layer.bindPopup(
                                `<div style="font-family:system-ui;min-width:140px">
                  <p style="font-weight:600;margin:0 0 4px">${escapedName}</p>
                  <p style="margin:0;color:#94a3b8;font-size:12px">${volume} pociągów/dobę</p>
                </div>`,
                            )
                        }}
                    />
                )}
            </MapContainer>

            <MapLegend />
        </div>
    )
}