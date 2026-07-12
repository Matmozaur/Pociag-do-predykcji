'use client'

import 'leaflet/dist/leaflet.css'
import { useEffect, useState } from 'react'
import { GeoJSON, MapContainer, TileLayer, useMap } from 'react-leaflet'
import type { Feature, Geometry } from 'geojson'
import { circleMarker, type Layer } from 'leaflet'
import {
    stationGeoJSON,
    type StationFeatureProperties,
} from '@/lib/api'

const POLAND_BOUNDS: [[number, number], [number, number]] = [
    [49.0, 14.1],
    [54.9, 24.3],
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

function BoundsLock() {
    const map = useMap()
    useEffect(() => {
        const zoom = map.getBoundsZoom(POLAND_BOUNDS)
        map.setMinZoom(zoom)
    }, [map])
    return null
}

export function TrafficMapClient() {
    const [maskFeature, setMaskFeature] = useState<GeoJSON.Feature | null>(null)

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
                bounds={POLAND_BOUNDS}
                maxBounds={POLAND_BOUNDS}
                maxBoundsViscosity={1.0}
                maxZoom={19}
                worldCopyJump={false}
                style={{ width: '100%', height: '100%' }}
                zoomControl={true}
            >
                <BoundsLock />
                <TileLayer
                    url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png"
                    attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> &copy; <a href="https://carto.com">CARTO</a>'
                    subdomains="abcd"
                    maxZoom={20}
                    noWrap={true}
                    bounds={POLAND_BOUNDS}
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
                    bounds={POLAND_BOUNDS}
                    keepBuffer={2}
                    updateWhenIdle={true}
                />

                {/* Dark mask covering everything outside Poland's border */}
                {maskFeature && (
                    <GeoJSON
                        key="poland-mask"
                        data={maskFeature as GeoJSON.Feature<GeoJSON.Geometry>}
                        style={() => ({
                            fillColor: '#0a0c14',
                            fillOpacity: 1,
                            color: '#0a0c14',
                            weight: 0,
                        })}
                        interactive={false}
                    />
                )}

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

            </MapContainer>
        </div>
    )
}