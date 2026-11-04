'use client'

import 'leaflet/dist/leaflet.css'
import L from 'leaflet'
import Link from 'next/link'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowRight, MapPin, TrainFront } from 'lucide-react'
import { CircleMarker, GeoJSON, MapContainer, Marker, Pane, Polyline, Popup, Rectangle, TileLayer, Tooltip, useMap, useMapEvents } from 'react-leaflet'
import { formatDelay, api, type MapStation, type TrainMapPoint, type TrainMapStop } from '@/lib/api'
import type { RailIndex } from '@/lib/railSnap'
import { snapTrainPosition, useOverlapOffsets, useRailIndex } from '@/lib/useRailSnap'
import { useStationClusters, type StationMapItem } from '@/lib/useStationClusters'
import { useTrainRoute } from '@/lib/useTrainRoute'
import { Spinner, cn } from '@/lib/ui'

// Padded bounding box around Poland — the initial view is fitted to it.
const POLAND_VIEW_BOUNDS: L.LatLngBoundsExpression = [
    [48.85, 13.85],
    [54.98, 24.4],
]
// Panning is locked to roughly Central Europe: enough to follow lines across the border
// into the neighbouring networks, without scrolling out into an empty void.
const MAX_BOUNDS = L.latLngBounds([
    [45, 5],
    [58, 32],
])

const POLAND_FILL_STYLE: L.PathOptions = {
    fillColor: '#10141f',
    fillOpacity: 1,
    color: '#334155',
    weight: 1.2,
}

// Everything outside Poland is veiled over the railway tiles in the map background
// colour (see `.leaflet-container` in globals.css): foreign lines stay visible as dimmed
// background context (no train data exists for them), Polish ones stay the focus.
const OUTSIDE_POLAND_STYLE: L.PathOptions = {
    fillColor: '#1a1d27',
    fillOpacity: 0.6,
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

// Stations are background context under the trains: small, unstroked, dimmed dots in a
// muted sky tone that can't be mistaken for a train's delay colour.
const STATION_RADIUS = 4
const STATION_PATH_OPTIONS: L.PathOptions = {
    stroke: false,
    fillColor: '#7dd3fc',
    fillOpacity: 0.7,
}
// Zoomed out below the label zoom, ~3k stations overlap along every line, so draw them
// smaller and fainter there to keep the trains readable.
const OVERVIEW_STATION_RADIUS = 2.5
const OVERVIEW_STATION_PATH_OPTIONS: L.PathOptions = { ...STATION_PATH_OPTIONS, fillOpacity: 0.45 }
// While a train is selected the stations recede with the rest of the map.
const DIMMED_STATION_PATH_OPTIONS: L.PathOptions = { ...STATION_PATH_OPTIONS, fillOpacity: 0.2 }
const DIMMED_OVERVIEW_STATION_PATH_OPTIONS: L.PathOptions = { ...STATION_PATH_OPTIONS, fillOpacity: 0.12 }
// Nearby stations merge into count bubbles: a deeper shade of the station sky tone, opaque
// enough for the number on top to read. The radius grows with the square root of the count.
const CLUSTER_MIN_RADIUS = 9
const CLUSTER_MAX_RADIUS = 26
const CLUSTER_BASE_RADIUS = 5
const CLUSTER_RADIUS_PER_SQRT_COUNT = 2.4
const CLUSTER_PATH_OPTIONS: L.PathOptions = {
    stroke: true,
    color: '#7dd3fc',
    weight: 1,
    opacity: 0.7,
    fillColor: '#0284c7',
    fillOpacity: 0.75,
}
const DIMMED_CLUSTER_PATH_OPTIONS: L.PathOptions = { ...CLUSTER_PATH_OPTIONS, opacity: 0.2, fillOpacity: 0.2 }
const CLUSTER_FLY_DURATION_S = 0.6
const clusterRadius = (count: number) =>
    Math.min(CLUSTER_MAX_RADIUS, Math.max(CLUSTER_MIN_RADIUS, CLUSTER_BASE_RADIUS + CLUSTER_RADIUS_PER_SQRT_COUNT * Math.sqrt(count)))

// Real-world track geometry from OpenRailwayMap (OSM data, CC-BY-SA). The tiles are
// transparent, so they overlay the dark Poland fill without needing a base map. Dimmed
// so the network reads as context and the markers stay the focal layer; outside Poland
// they are dimmed further by the outside mask.
const RAILWAY_TILES_URL = 'https://{s}.tiles.openrailwaymap.org/standard/{z}/{x}/{y}.png'
const RAILWAY_TILES_ATTRIBUTION =
    'Data &copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors, ' +
    'style: <a href="https://www.openrailwaymap.org/">OpenRailwayMap</a> (CC-BY-SA)'

// Geographic context from public/geo-background.geojson (Natural Earth, public domain;
// built by scripts/build-geo-background.mjs): lakes in a muted navy just above the Poland
// fill, and river lines in a brighter water blue. All of it, city names included, sits
// UNDER the railway tiles — geography is background, the rails are always drawn over it;
// the tiles are translucent, so a river sharing its valley with a main line still reads
// beside it. Both stay darker and more saturated than the sky-blue station dots and far
// from the delay colours. City names are small and grey. Rank 1 is the major rivers /
// voivodeship seats and capitals; minor city names appear from CITY_MINOR_MIN_ZOOM and all
// of them give way to station labels from STATION_LABEL_MIN_ZOOM.
type GeoBackgroundFeature = GeoJSON.Feature<GeoJSON.Geometry, { kind: 'river' | 'lake' | 'city'; rank: 1 | 2; name?: string }>
type GeoBackground = GeoJSON.FeatureCollection<GeoJSON.Geometry, GeoBackgroundFeature['properties']>
const WATER_COLOR = '#2f62a0'
const LAKE_STYLE: L.PathOptions = { color: WATER_COLOR, weight: 1, fillColor: WATER_COLOR, fillOpacity: 0.85 }
const RIVER_COLOR = '#3d8bff'
const MAJOR_RIVER_STYLE: L.PathOptions = { color: RIVER_COLOR, weight: 3.5, opacity: 0.85, lineJoin: 'round', lineCap: 'round' }
const MINOR_RIVER_STYLE: L.PathOptions = { color: RIVER_COLOR, weight: 1.6, opacity: 0.6, lineJoin: 'round', lineCap: 'round' }
const riverStyle = (feature?: GeoJSON.Feature): L.PathOptions =>
    (feature as GeoBackgroundFeature | undefined)?.properties.rank === 1 ? MAJOR_RIVER_STYLE : MINOR_RIVER_STYLE
const CITY_MINOR_MIN_ZOOM = 7

// Selected station: a light ring around a soft fill — clearly picked out, but no larger
// than a train dot and not amber (that's the 1–5 min delay colour). Still painted under
// the trains on the shared canvas, so trains keep their clicks.
const SELECTED_STATION_RADIUS = 6
const SELECTED_STATION_PATH_OPTIONS: L.PathOptions = {
    stroke: true, // setStyle merges, so re-enable what STATION_PATH_OPTIONS turned off
    color: '#bae6fd',
    weight: 1.5,
    opacity: 0.9,
    fillColor: '#38bdf8',
    fillOpacity: 0.35,
}
// The same station while it sits inside a cluster: the soft ring vanishes on the bubble, so
// here it's the one station bigger than a train dot, opaque and cased like the trains. Fuchsia
// because the near-white count digits can land on it, which rules out a light fill.
const PINNED_STATION_RADIUS = 8
const PINNED_STATION_PATH_OPTIONS: L.PathOptions = {
    stroke: true, // setStyle merges, so re-enable what STATION_PATH_OPTIONS turned off
    color: '#0a0c14',
    weight: 2,
    opacity: 1,
    fillColor: '#c026d3',
    fillOpacity: 1,
}
const DIMMED_PINNED_STATION_PATH_OPTIONS: L.PathOptions = { ...PINNED_STATION_PATH_OPTIONS, opacity: 0.3, fillOpacity: 0.3 }

// Selected train: everything but the train and its whole route goes behind a veil in the
// map background colour, and the route is drawn as a light line on a dark casing — neither
// a delay colour nor the sky blue of the station dots. The veil covers the whole world so
// no edge shows at minimum zoom.
const SELECTION_VEIL_BOUNDS = L.latLngBounds([
    [-85, -180],
    [85, 180],
])
const SELECTION_VEIL_STYLE: L.PathOptions = { stroke: false, fillColor: '#12151e', fillOpacity: 0.55 }
const ROUTE_CASING_STYLE: L.PathOptions = { color: '#0a0c14', weight: 7, opacity: 0.85, lineJoin: 'round', lineCap: 'round' }
const ROUTE_LINE_STYLE: L.PathOptions = { color: '#f1f5f9', weight: 3.5, opacity: 0.95, lineJoin: 'round', lineCap: 'round' }

// Permanent station labels only from this zoom up — with ~3k stations they are
// unreadable (and slow, one DOM node each) on the country-wide view.
const STATION_LABEL_MIN_ZOOM = 9

const TRAINS_REFETCH_MS = 30_000
const TRAINS_STALE_MS = 20_000
// How often en-route trains are re-interpolated client-side between polls.
const TRAIN_TICK_MS = 5_000

const warsawClock = new Intl.DateTimeFormat('pl-PL', {
    timeZone: 'Europe/Warsaw',
    hour: '2-digit',
    minute: '2-digit',
})

function formatClock(instant?: string): string {
    if (!instant) return '—'
    const date = new Date(instant)
    return Number.isNaN(date.getTime()) ? '—' : warsawClock.format(date)
}

type DelayTone = 'onTime' | 'minor' | 'major' | 'unknown'

function delayTone(minutes?: number): DelayTone {
    if (minutes == null) return 'unknown'
    if (minutes <= 0) return 'onTime'
    if (minutes <= 5) return 'minor'
    return 'major'
}

const DELAY_COLORS: Record<DelayTone, string> = {
    onTime: '#22c55e',
    minor: '#f59e0b',
    major: '#ef4444',
    unknown: '#94a3b8',
}

const DELAY_TEXT_CLASS: Record<DelayTone, string> = {
    onTime: 'text-emerald-300',
    minor: 'text-amber-300',
    major: 'text-red-300',
    unknown: 'text-slate-400',
}

const LEGEND: { tone: DelayTone; label: string }[] = [
    { tone: 'onTime', label: 'Na czas' },
    { tone: 'minor', label: '1–5 min' },
    { tone: 'major', label: 'Ponad 5 min' },
    { tone: 'unknown', label: 'Brak danych' },
]

// Pre-built style objects so react-leaflet only calls setStyle when a train's colour
// or confidence actually changes, not on every 5 s position tick.
// Dimmed: another train is selected, so this one fades into the background.
function buildTrainStyle(tone: DelayTone, lowConfidence: boolean, selected: boolean, dimmed: boolean): L.PathOptions {
    return {
        color: selected ? '#f8fafc' : '#0a0c14',
        weight: selected ? 2 : 1.25,
        opacity: dimmed ? 0.25 : lowConfidence ? 0.45 : 1,
        fillColor: DELAY_COLORS[tone],
        fillOpacity: dimmed ? (lowConfidence ? 0.15 : 0.3) : lowConfidence ? 0.4 : 0.95,
    }
}
const TRAIN_STYLES = new Map<string, L.PathOptions>()
function trainStyle(tone: DelayTone, lowConfidence: boolean, selected: boolean, dimmed: boolean): L.PathOptions {
    const key = `${tone}:${lowConfidence}:${selected}:${dimmed}`
    let style = TRAIN_STYLES.get(key)
    if (!style) {
        style = buildTrainStyle(tone, lowConfidence, selected, dimmed)
        TRAIN_STYLES.set(key, style)
    }
    return style
}

function hasCoords(stop?: TrainMapStop): stop is TrainMapStop & { latitude: number; longitude: number } {
    return stop?.latitude != null && stop.longitude != null
}

// Wall-clock share (0–1, clamped) of an en-route train's run between its stops, or null
// whenever the segment can't be interpolated.
function segmentFraction(train: TrainMapPoint, now: number): number | null {
    const { previous_stop: prev, next_stop: next } = train
    if (train.phase !== 'en_route' || !hasCoords(prev) || !hasCoords(next)) return null
    const start = Date.parse(prev.time)
    const end = Date.parse(next.time)
    if (!Number.isFinite(start) || !Number.isFinite(end) || end <= start) return null
    return Math.min(1, Math.max(0, (now - start) / (end - start)))
}

// Linear interpolation between the segment's stops at fraction f. Falls back to the
// server position whenever the segment can't be lerped.
function estimatePosition(train: TrainMapPoint, f: number | null): [number, number] {
    const { previous_stop: prev, next_stop: next } = train
    if (f == null || !hasCoords(prev) || !hasCoords(next)) return [train.latitude, train.longitude]
    return [prev.latitude + (next.latitude - prev.latitude) * f, prev.longitude + (next.longitude - prev.longitude) * f]
}

function pluralPl(n: number, one: string, few: string, many: string): string {
    if (n === 1) return one
    const last = n % 10
    const lastTwo = n % 100
    return last >= 2 && last <= 4 && !(lastTwo >= 12 && lastTwo <= 14) ? few : many
}

const PHASE_LABELS: Record<TrainMapPoint['phase'], string> = {
    not_departed: 'Przed odjazdem',
    at_station: 'Na stacji',
    en_route: 'W drodze',
    arrived: 'Przyjechał',
}

interface City {
    name: string
    rank: 1 | 2
    position: L.LatLngTuple
    icon: L.DivIcon
}

const escapeHtml = (text: string) => text.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`)

// Dot + name, styled by .city-label in globals.css (anchored at the dot's centre).
function cityIcon(name: string, rank: 1 | 2): L.DivIcon {
    return L.divIcon({
        className: rank === 1 ? 'city-label' : 'city-label city-label--minor',
        iconSize: [0, 0],
        html: `<span>${escapeHtml(name)}</span>`,
    })
}

// Non-interactive city names in placeLabelsPane; minor ones only once zoomed in, and all
// hidden at station-label zoom where station names take over.
function CityLabels({ cities }: { cities: City[] }) {
    const map = useMap()
    const [zoom, setZoom] = useState(() => map.getZoom())
    useMapEvents({ zoomend: () => setZoom(map.getZoom()) })
    if (zoom >= STATION_LABEL_MIN_ZOOM) return null
    return (
        <>
            {cities
                .filter((c) => c.rank === 1 || zoom >= CITY_MINOR_MIN_ZOOM)
                .map((c) => (
                    <Marker key={c.name} position={c.position} icon={c.icon} interactive={false} keyboard={false} pane="placeLabelsPane" />
                ))}
        </>
    )
}

// Pans (without changing zoom) to the selected station whenever the selection changes.
function PanToStation({ station }: { station?: MapStation }) {
    const map = useMap()
    useEffect(() => {
        if (station) map.panTo([station.latitude, station.longitude], { animate: true })
    }, [map, station])
    return null
}

interface StationMarkerProps {
    station: MapStation
    selected: boolean
    pinned: boolean
    labelled: boolean
    overview: boolean
    dimmed: boolean
    renderer: L.Renderer
    onSelect: (station: MapStation) => void
}

const StationMarker = memo(function StationMarker({ station, selected, pinned, labelled, overview, dimmed, renderer, onSelect }: StationMarkerProps) {
    const center = useMemo<L.LatLngTuple>(() => [station.latitude, station.longitude], [station.latitude, station.longitude])
    const eventHandlers = useMemo(() => ({ click: () => onSelect(station) }), [onSelect, station])
    const markerRef = useRef<L.CircleMarker>(null)
    // Stations mount on zoom changes, after the trains: canvas paint and hit-test order is add
    // order, so without this they would cover the trains and steal their clicks.
    useEffect(() => {
        markerRef.current?.bringToBack()
    }, [])
    return (
        <CircleMarker
            ref={markerRef}
            center={center}
            radius={pinned ? PINNED_STATION_RADIUS : selected ? SELECTED_STATION_RADIUS : overview ? OVERVIEW_STATION_RADIUS : STATION_RADIUS}
            renderer={renderer}
            pathOptions={
                pinned
                    ? dimmed
                        ? DIMMED_PINNED_STATION_PATH_OPTIONS
                        : PINNED_STATION_PATH_OPTIONS
                    : selected
                      ? SELECTED_STATION_PATH_OPTIONS
                      : dimmed
                        ? overview
                            ? DIMMED_OVERVIEW_STATION_PATH_OPTIONS
                            : DIMMED_STATION_PATH_OPTIONS
                        : overview
                          ? OVERVIEW_STATION_PATH_OPTIONS
                          : STATION_PATH_OPTIONS
            }
            eventHandlers={eventHandlers}
        >
            {labelled ? (
                <Tooltip
                    key="label"
                    direction="top"
                    offset={[0, -4]}
                    opacity={1}
                    permanent
                    pane="stationLabelsPane"
                    className="station-label-tooltip"
                >
                    <span className="font-semibold">{station.name}</span>
                </Tooltip>
            ) : (
                <Tooltip key="hover" direction="top" offset={[0, -4]} opacity={1} className="station-label-tooltip">
                    <span className="font-semibold">{station.name}</span>
                </Tooltip>
            )}
        </CircleMarker>
    )
})

interface ClusterMarkerProps {
    latitude: number
    longitude: number
    count: number
    expansionZoom: number
    // Near the viewport. Each count is a DOM node that Leaflet repositions on every frame of a
    // flight, so off-screen ones would make zooming stutter.
    countShown: boolean
    dimmed: boolean
    renderer: L.Renderer
    // Returns the matching unregister function.
    onMount: (marker: L.CircleMarker) => () => void
}

const ClusterMarker = memo(function ClusterMarker({ latitude, longitude, count, expansionZoom, countShown, dimmed, renderer, onMount }: ClusterMarkerProps) {
    const map = useMap()
    const markerRef = useRef<L.CircleMarker>(null)
    const center = useMemo<L.LatLngTuple>(() => [latitude, longitude], [latitude, longitude])
    const eventHandlers = useMemo(
        () => ({ click: () => map.flyTo(center, expansionZoom, { duration: CLUSTER_FLY_DURATION_S }) }),
        [map, center, expansionZoom],
    )
    // Same reason as StationMarker; StationsLayer then sends clusters below the stations too.
    useEffect(() => {
        const marker = markerRef.current
        if (!marker) return
        marker.bringToBack()
        return onMount(marker)
    }, [onMount])
    return (
        <CircleMarker
            ref={markerRef}
            center={center}
            radius={clusterRadius(count)}
            renderer={renderer}
            pathOptions={dimmed ? DIMMED_CLUSTER_PATH_OPTIONS : CLUSTER_PATH_OPTIONS}
            eventHandlers={eventHandlers}
        >
            {/* Keyed because react-leaflet doesn't update a tooltip's class. In its own pane above
                the canvas: stationLabelsPane is under it, so the circle would cover the number. */}
            {countShown && (
                <Tooltip
                    key={dimmed ? 'dimmed' : 'normal'}
                    direction="center"
                    offset={[0, 0]}
                    opacity={1}
                    permanent
                    pane="clusterCountsPane"
                    className={dimmed ? 'station-cluster-count station-cluster-count--dimmed' : 'station-cluster-count'}
                >
                    {count}
                </Tooltip>
            )}
        </CircleMarker>
    )
})

interface StationsLayerProps {
    stations: MapStation[]
    selectedStationId?: number
    dimmed: boolean
    renderer: L.Renderer
    onStationSelect?: (station: MapStation) => void
}

function StationsLayer({ stations, selectedStationId, dimmed, renderer, onStationSelect }: StationsLayerProps) {
    const map = useMap()
    const [view, setView] = useState(() => ({ zoom: map.getZoom(), bounds: map.getBounds().pad(0.2) }))
    useMapEvents({
        moveend: () => setView({ zoom: map.getZoom(), bounds: map.getBounds().pad(0.2) }),
    })

    // Keep the click handler stable so memoised markers don't re-render when the
    // parent passes a fresh callback.
    const onSelectRef = useRef(onStationSelect)
    useEffect(() => {
        onSelectRef.current = onStationSelect
    }, [onStationSelect])
    const handleSelect = useCallback((station: MapStation) => onSelectRef.current?.(station), [])

    const items = useStationClusters(stations, view.zoom)
    // The selected station is always its own dot, even while it sits in a cluster. Appended to
    // the same list under the same key, so it doesn't remount going in and out of a cluster.
    const drawnItems = useMemo<StationMapItem[]>(() => {
        if (selectedStationId == null) return items
        if (items.some((item) => item.kind === 'station' && item.station.external_id === selectedStationId)) return items
        const selected = stations.find((station) => station.external_id === selectedStationId)
        return selected ? [...items, { kind: 'station', station: selected }] : items
    }, [items, stations, selectedStationId])

    const clusterMarkers = useRef(new Set<L.CircleMarker>())
    const registerClusterMarker = useCallback((marker: L.CircleMarker) => {
        const markers = clusterMarkers.current
        markers.add(marker)
        return () => {
            markers.delete(marker)
        }
    }, [])
    // Runs after the markers' own mount effects, which have just sent new stations (and a newly
    // pinned selected one) below the clusters. Final order: clusters, stations, trains.
    useEffect(() => {
        clusterMarkers.current.forEach((marker) => marker.bringToBack())
    }, [items, selectedStationId])

    const showLabels = view.zoom >= STATION_LABEL_MIN_ZOOM
    return (
        <>
            {drawnItems.map((item) =>
                item.kind === 'cluster' ? (
                    <ClusterMarker
                        key={`c${item.id}`}
                        latitude={item.latitude}
                        longitude={item.longitude}
                        count={item.count}
                        expansionZoom={item.expansionZoom}
                        countShown={view.bounds.contains([item.latitude, item.longitude])}
                        dimmed={dimmed}
                        renderer={renderer}
                        onMount={registerClusterMarker}
                    />
                ) : (
                    <StationMarker
                        key={`s${item.station.external_id}`}
                        station={item.station}
                        selected={item.station.external_id === selectedStationId}
                        pinned={drawnItems !== items && item.station.external_id === selectedStationId}
                        labelled={showLabels && view.bounds.contains([item.station.latitude, item.station.longitude])}
                        overview={!showLabels}
                        dimmed={dimmed}
                        renderer={renderer}
                        onSelect={handleSelect}
                    />
                ),
            )}
        </>
    )
}

function StopLine({ label, stop }: { label: string; stop?: TrainMapStop }) {
    return (
        <div className="flex items-baseline justify-between gap-3">
            <dt className="sr-only">{label}</dt>
            <dd className="min-w-0 truncate text-slate-200">{stop?.station_name ?? 'Nieznana stacja'}</dd>
            <dd className="flex-none tabular-nums text-slate-400">{formatClock(stop?.time)}</dd>
        </div>
    )
}

function TrainPopupContent({ train, dataAsOf }: { train: TrainMapPoint; dataAsOf?: string }) {
    const tone = delayTone(train.delay_minutes)
    const route = train.origin && train.destination ? `${train.origin} → ${train.destination}` : undefined
    return (
        <div className="w-60 text-xs text-slate-300">
            <p className="text-sm font-semibold leading-snug text-white">{train.train_name}</p>
            <p className="mt-0.5 text-slate-400">
                {[train.carrier_code, PHASE_LABELS[train.phase]].filter(Boolean).join(' · ')}
            </p>
            {route && <p className="mt-1 truncate text-slate-500" title={route}>{route}</p>}

            {(train.previous_stop || train.next_stop) && (
                <dl className="mt-2.5 space-y-1 rounded-md border border-white/8 bg-black/20 px-2 py-1.5">
                    {train.previous_stop && <StopLine label="Poprzednia stacja" stop={train.previous_stop} />}
                    {train.previous_stop && train.next_stop && (
                        <div aria-hidden="true" className="pl-1 text-[10px] leading-none text-slate-600">↓</div>
                    )}
                    {train.next_stop && <StopLine label="Następna stacja" stop={train.next_stop} />}
                </dl>
            )}

            <div className="mt-2.5 flex items-center justify-between gap-3">
                <span>
                    Opóźnienie:{' '}
                    <span className={cn('font-semibold tabular-nums', DELAY_TEXT_CLASS[tone])}>
                        {train.delay_minutes == null ? 'brak danych' : formatDelay(train.delay_minutes)}
                    </span>
                </span>
                <span className="text-slate-500">dane z {formatClock(dataAsOf)}</span>
            </div>
            {train.confidence === 'low' && (
                <p className="mt-1.5 text-[11px] text-slate-500">Pozycja orientacyjna — brak świeżych potwierdzeń z trasy.</p>
            )}

            <Link
                href={`/pociagi/${train.operation_id}`}
                className="mt-3 inline-flex items-center gap-1 font-medium !text-blue-300 hover:!text-blue-200 focus-visible:rounded focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400"
            >
                Szczegóły pociągu <ArrowRight size={13} aria-hidden="true" />
            </Link>
        </div>
    )
}

// Keep auto-panned popups clear of the overlays drawn over the map: the operations panel
// (left on desktop, bottom sheet on mobile), the search bar (top) and the layers panel
// (top right).
function popupPanPadding(): { topLeft: L.PointTuple; bottomRight: L.PointTuple } {
    if (window.innerWidth >= 768) return { topLeft: [430, 76], bottomRight: [260, 24] }
    return { topLeft: [12, 72], bottomRight: [12, Math.round(window.innerHeight * 0.5)] }
}

interface TrainMarkerProps {
    train: TrainMapPoint
    latitude: number
    longitude: number
    dataAsOf?: string
    renderer: L.Renderer
    // Another train is selected (its popup is open).
    dimmed: boolean
    // Set only on the train picked in search (undefined for every other marker, so memo holds).
    focusNonce?: number
    onFocusHandled: (nonce: number) => void
    onOpenChange: (operationId: number, open: boolean) => void
}

const TrainMarker = memo(function TrainMarker({
    train,
    latitude,
    longitude,
    dataAsOf,
    renderer,
    dimmed,
    focusNonce,
    onFocusHandled,
    onOpenChange,
}: TrainMarkerProps) {
    const map = useMap()
    const markerRef = useRef<L.CircleMarker>(null)
    const [open, setOpen] = useState(false)
    const center = useMemo<L.LatLngTuple>(() => [latitude, longitude], [latitude, longitude])
    const padding = useMemo(popupPanPadding, [])
    const operationId = train.operation_id
    const eventHandlers = useMemo(
        () => ({
            popupopen: () => {
                setOpen(true)
                onOpenChange(operationId, true)
            },
            popupclose: () => {
                setOpen(false)
                onOpenChange(operationId, false)
            },
        }),
        [onOpenChange, operationId],
    )

    // Draw the selected train over the others on the shared canvas; if the marker goes away
    // with its popup open (refetch without it, trains layer hidden), drop the selection too.
    useEffect(() => {
        if (!open) return
        markerRef.current?.bringToFront()
        return () => onOpenChange(operationId, false)
    }, [open, onOpenChange, operationId])

    // Fly to the train's drawn position and open its popup once the flight ends. Keyed on
    // the nonce alone so interpolation ticks don't re-fly; the nonce is marked handled only
    // after the popup opens, so a remount mid-flight simply starts over.
    useEffect(() => {
        const marker = markerRef.current
        if (focusNonce == null || !marker) return
        const onMoveEnd = () => {
            marker.openPopup()
            onFocusHandled(focusNonce)
        }
        // flyTo first: it synchronously stops any running pan, which fires its own moveend.
        map.flyTo(marker.getLatLng(), Math.max(map.getZoom(), 10), { duration: 0.8 })
        map.once('moveend', onMoveEnd)
        return () => {
            map.off('moveend', onMoveEnd)
        }
    }, [focusNonce, map, onFocusHandled])

    return (
        <CircleMarker
            ref={markerRef}
            center={center}
            radius={open ? 8.5 : 6.5}
            renderer={renderer}
            pathOptions={trainStyle(delayTone(train.delay_minutes), train.confidence === 'low', open, dimmed)}
            eventHandlers={eventHandlers}
        >
            {/* Explicit pane: a Popup otherwise inherits trainsPane and the canvas dots paint over it. */}
            <Popup
                pane="popupPane"
                minWidth={240}
                maxWidth={280}
                autoPanPaddingTopLeft={padding.topLeft}
                autoPanPaddingBottomRight={padding.bottomRight}
            >
                <TrainPopupContent train={train} dataAsOf={dataAsOf} />
            </Popup>
        </CircleMarker>
    )
})

interface TrainsLayerProps {
    trains: TrainMapPoint[]
    dataAsOf?: string
    now: number | null
    railIndex: RailIndex | null
    renderer: L.Renderer
    selectedTrainId?: number
    focus?: { id: number; nonce: number }
    onFocusHandled: (nonce: number) => void
    onTrainOpenChange: (operationId: number, open: boolean) => void
}

function TrainsLayer({
    trains,
    dataAsOf,
    now,
    railIndex,
    renderer,
    selectedTrainId,
    focus,
    onFocusHandled,
    onTrainOpenChange,
}: TrainsLayerProps) {
    // The single place each train's drawn position is decided: place it along the railway
    // route between its stops (or snap onto the nearest line), then slide trains that would
    // overlap on screen apart along their tracks.
    const points = useMemo(
        () =>
            trains.map((train) => {
                const fraction = now == null ? null : segmentFraction(train, now)
                const { position, bearing, track } = snapTrainPosition(railIndex, train, estimatePosition(train, fraction), fraction)
                return { id: train.operation_id, latitude: position[0], longitude: position[1], bearing, track }
            }),
        [trains, now, railIndex],
    )
    const spread = useOverlapOffsets(points)
    return (
        <>
            {trains.map((train, i) => {
                const [latitude, longitude] = spread.get(train.operation_id) ?? [points[i].latitude, points[i].longitude]
                return (
                    <TrainMarker
                        key={train.operation_id}
                        train={train}
                        latitude={latitude}
                        longitude={longitude}
                        dataAsOf={dataAsOf}
                        renderer={renderer}
                        dimmed={selectedTrainId != null && selectedTrainId !== train.operation_id}
                        focusNonce={focus?.id === train.operation_id ? focus.nonce : undefined}
                        onFocusHandled={onFocusHandled}
                        onOpenChange={onTrainOpenChange}
                    />
                )
            })}
        </>
    )
}

function LayerToggle({
    pressed,
    onToggle,
    icon,
    children,
}: {
    pressed: boolean
    onToggle: () => void
    icon: React.ReactNode
    children: React.ReactNode
}) {
    return (
        <button
            type="button"
            aria-pressed={pressed}
            onClick={onToggle}
            className={cn(
                'inline-flex items-center justify-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400',
                pressed ? 'bg-white/10 text-white' : 'text-slate-500 hover:text-slate-200',
            )}
        >
            {icon}
            {children}
        </button>
    )
}

interface TrafficMapClientProps {
    selectedStationId?: number
    onStationSelect?: (station: MapStation) => void
    // Train picked in search; a new nonce re-focuses even when the same train is picked again.
    focusedTrain?: { id: number; nonce: number }
}

export function TrafficMapClient({ selectedStationId, onStationSelect, focusedTrain }: TrafficMapClientProps = {}) {
    const [polandGeo, setPolandGeo] = useState<PolandFeature | null>(null)
    const [geoBackground, setGeoBackground] = useState<GeoBackground | null>(null)
    const [showStations, setShowStations] = useState(true)
    const [showTrains, setShowTrains] = useState(true)

    // One canvas for both stations and trains: a Leaflet canvas swallows pointer events
    // for everything underneath it, so two stacked canvases would leave the lower layer
    // unclickable. It lives in trainsPane (above stationsPane); trains are drawn last, as
    // station and cluster markers send themselves to the back when they mount.
    const pointsRenderer = useMemo(() => L.canvas({ pane: 'trainsPane', padding: 0.5, tolerance: 4 }), [])

    const { data: stationsData } = useQuery({
        queryKey: ['mapStations'],
        queryFn: api.getMapStations,
        staleTime: 60 * 60 * 1000,
    })

    const trainsQuery = useQuery({
        queryKey: ['mapTrains'],
        queryFn: () => api.getMapTrains(),
        refetchInterval: TRAINS_REFETCH_MS,
        staleTime: TRAINS_STALE_MS,
        enabled: showTrains,
    })
    const trainsData = trainsQuery.data

    // Interpolation clock. A tick older than the latest response is ignored, so fresh
    // data is placed as of its fetch time until the next tick moves it on — still along
    // the railway route, so trains don't jump off their line after every refetch.
    const [tick, setTick] = useState<number | null>(null)
    useEffect(() => {
        if (!showTrains) return
        const id = window.setInterval(() => setTick(Date.now()), TRAIN_TICK_MS)
        return () => window.clearInterval(id)
    }, [showTrains])
    const now = tick != null && tick > trainsQuery.dataUpdatedAt ? tick : trainsQuery.dataUpdatedAt || null
    // Railway line geometry for snapping; only fetched once trains are shown.
    const railIndex = useRailIndex(showTrains)

    // The train whose popup is open. Opening popup B closes popup A first, so a close only
    // clears the selection when it is still the closing train's.
    const [selectedTrainId, setSelectedTrainId] = useState<number | undefined>(undefined)
    const handleTrainOpenChange = useCallback((operationId: number, open: boolean) => {
        setSelectedTrainId((current) => (open ? operationId : current === operationId ? undefined : current))
    }, [])

    // A train picked in search turns the trains layer back on; the focus stays pending until
    // its marker has flown there (handles the trains still loading), and is then dropped so
    // a TrainsLayer remount doesn't fly to it again.
    const [handledFocusNonce, setHandledFocusNonce] = useState<number | null>(null)
    const focusNonce = focusedTrain?.nonce
    useEffect(() => {
        if (focusNonce != null) setShowTrains(true)
    }, [focusNonce])
    const focusUnhandled = focusedTrain != null && focusedTrain.nonce !== handledFocusNonce
    const focusInData = focusedTrain != null && !!trainsData?.trains.some((t) => t.operation_id === focusedTrain.id)
    const pendingFocus = focusUnhandled && focusInData ? focusedTrain : undefined
    // Fresh data without the picked train drops the focus, so a later refetch that brings the
    // train back doesn't fly the map unprompted.
    const dropFocus = focusUnhandled && trainsData != null && !trainsQuery.isFetching && !focusInData
    useEffect(() => {
        if (dropFocus && focusNonce != null) setHandledFocusNonce(focusNonce)
    }, [dropFocus, focusNonce])

    useEffect(() => {
        fetch('/poland-border.geojson')
            .then((r) => r.json())
            .then((geojson: PolandFeature) => setPolandGeo(geojson))
            .catch(() => {
                // border shape not critical to interactivity — fail silently
            })
    }, [])

    useEffect(() => {
        fetch('/geo-background.geojson')
            .then((r) => r.json())
            .then((geojson: GeoBackground) => setGeoBackground(geojson))
            .catch(() => {
                // geographic context is decoration — fail silently
            })
    }, [])

    const lakes = useMemo<GeoBackground | null>(
        () => (geoBackground ? { ...geoBackground, features: geoBackground.features.filter((f) => f.properties.kind === 'lake') } : null),
        [geoBackground],
    )
    const rivers = useMemo<GeoBackground | null>(
        () => (geoBackground ? { ...geoBackground, features: geoBackground.features.filter((f) => f.properties.kind === 'river') } : null),
        [geoBackground],
    )
    const cities = useMemo<City[]>(
        () =>
            (geoBackground?.features ?? []).flatMap((f) =>
                f.properties.kind === 'city' && f.geometry.type === 'Point' && f.properties.name
                    ? [
                          {
                              name: f.properties.name,
                              rank: f.properties.rank,
                              position: [f.geometry.coordinates[1], f.geometry.coordinates[0]] as L.LatLngTuple,
                              icon: cityIcon(f.properties.name, f.properties.rank),
                          },
                      ]
                    : [],
            ),
        [geoBackground],
    )

    const outsideMask = useMemo(() => (polandGeo ? buildOutsideMask(polandGeo) : null), [polandGeo])
    const selectedStation = stationsData?.stations.find((s) => s.external_id === selectedStationId)
    const selectedRoute = useTrainRoute(selectedTrainId, stationsData?.stations, railIndex)
    const trainSelected = selectedTrainId != null
    const stationsDrawn = showStations && stationsData != null

    const trainCount = trainsData?.trains.length ?? 0
    const unpositioned = trainsData?.unpositioned_count ?? 0

    return (
        <div className="relative h-full w-full">
            <MapContainer
                bounds={POLAND_VIEW_BOUNDS}
                maxBounds={MAX_BOUNDS}
                maxBoundsViscosity={1.0}
                minZoom={5}
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

                {/* Explicit panes pin the paint order: Poland fill (overlayPane, z 400) → lakes →
                    rivers → city names → railway tiles → outside-Poland veil → station labels →
                    selected-train veil → selected route → the canvas (clusters, station dots, then
                    trains) → cluster counts. All geography stays under the rails. */}
                <Pane name="waterPane" style={{ zIndex: 405, pointerEvents: 'none' }}>
                    {lakes && <GeoJSON data={lakes} style={LAKE_STYLE} interactive={false} />}
                </Pane>

                <Pane name="riversPane" style={{ zIndex: 407, pointerEvents: 'none' }}>
                    {rivers && <GeoJSON data={rivers} style={riverStyle} interactive={false} />}
                </Pane>

                <Pane name="placeLabelsPane" style={{ zIndex: 408, pointerEvents: 'none' }}>
                    <CityLabels cities={cities} />
                </Pane>

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
                    {stationsDrawn && (
                        <StationsLayer
                            stations={stationsData.stations}
                            selectedStationId={selectedStationId}
                            dimmed={trainSelected}
                            renderer={pointsRenderer}
                            onStationSelect={onStationSelect}
                        />
                    )}
                </Pane>

                {/* Permanent station labels sit under the trains (the default tooltip pane is z 650). */}
                <Pane name="stationLabelsPane" style={{ zIndex: 425 }} />

                {/* Station dots and other trains share the canvas above, so they are dimmed
                    through their own styles instead. */}
                <Pane name="selectionVeilPane" style={{ zIndex: 426, pointerEvents: 'none' }}>
                    {trainSelected && (
                        <Rectangle bounds={SELECTION_VEIL_BOUNDS} pathOptions={SELECTION_VEIL_STYLE} interactive={false} />
                    )}
                </Pane>

                <Pane name="selectedRoutePane" style={{ zIndex: 428, pointerEvents: 'none' }}>
                    {trainSelected && selectedRoute && (
                        <>
                            <Polyline positions={selectedRoute} pathOptions={ROUTE_CASING_STYLE} interactive={false} />
                            <Polyline positions={selectedRoute} pathOptions={ROUTE_LINE_STYLE} interactive={false} />
                        </>
                    )}
                </Pane>

                <Pane name="trainsPane" style={{ zIndex: 430 }}>
                    {showTrains && trainsData && (
                        // Remount when stations (re)appear so trains are re-added to the
                        // shared canvas after them and stay drawn — and clickable — on top.
                        <TrainsLayer
                            key={stationsDrawn ? 'over-stations' : 'alone'}
                            trains={trainsData.trains}
                            dataAsOf={trainsData.data_as_of}
                            now={now}
                            railIndex={railIndex}
                            renderer={pointsRenderer}
                            selectedTrainId={selectedTrainId}
                            focus={pendingFocus}
                            onFocusHandled={setHandledFocusNonce}
                            onTrainOpenChange={handleTrainOpenChange}
                        />
                    )}
                </Pane>
                {/* Numbers on the canvas's cluster circles; clicks pass through to the canvas. */}
                <Pane name="clusterCountsPane" style={{ zIndex: 431, pointerEvents: 'none' }} />
                <PanToStation station={selectedStation} />
            </MapContainer>

            <div className="absolute right-3 top-[4.25rem] z-[450] w-56 rounded-xl border border-white/10 bg-[#121622]/90 p-2.5 text-xs text-slate-300 shadow-lg shadow-black/30 backdrop-blur md:right-4 xl:top-4">
                <div role="group" aria-label="Warstwy mapy" className="grid grid-cols-2 gap-1 rounded-lg bg-black/25 p-1">
                    <LayerToggle pressed={showStations} onToggle={() => setShowStations((v) => !v)} icon={<MapPin size={13} aria-hidden="true" />}>
                        Stacje
                    </LayerToggle>
                    <LayerToggle pressed={showTrains} onToggle={() => setShowTrains((v) => !v)} icon={<TrainFront size={13} aria-hidden="true" />}>
                        Pociągi
                    </LayerToggle>
                </div>

                {showTrains && (
                    <div className="mt-2.5 px-1" aria-live="polite">
                        {trainsQuery.isPending ? (
                            <div className="flex items-center gap-2 text-slate-400">
                                <Spinner className="h-3.5 w-3.5" /> Ładowanie pociągów…
                            </div>
                        ) : trainsData == null ? (
                            <div role="alert" className="text-slate-300">
                                <p>Nie udało się pobrać pozycji pociągów.</p>
                                <button
                                    type="button"
                                    onClick={() => void trainsQuery.refetch()}
                                    className="mt-1 font-semibold text-red-300 underline underline-offset-4 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400"
                                >
                                    Spróbuj ponownie
                                </button>
                            </div>
                        ) : (
                            <>
                                <p className="font-medium tabular-nums text-slate-200">
                                    {trainCount} {pluralPl(trainCount, 'pociąg', 'pociągi', 'pociągów')}, {unpositioned} bez pozycji
                                </p>
                                <p className="mt-0.5 text-[11px] text-slate-500">
                                    dane z {formatClock(trainsData.data_as_of)}
                                    {trainsQuery.isError && <span className="text-amber-300"> · nie udało się odświeżyć</span>}
                                </p>
                                <ul aria-label="Legenda opóźnień" className="mt-2 grid grid-cols-2 gap-x-2 gap-y-1 border-t border-white/8 pt-2 text-[11px] text-slate-400">
                                    {LEGEND.map(({ tone, label }) => (
                                        <li key={tone} className="flex items-center gap-1.5">
                                            <span aria-hidden="true" className="h-2.5 w-2.5 flex-none rounded-full ring-1 ring-black/60" style={{ backgroundColor: DELAY_COLORS[tone] }} />
                                            {label}
                                        </li>
                                    ))}
                                    <li className="col-span-2 flex items-center gap-1.5">
                                        <span aria-hidden="true" className="h-2.5 w-2.5 flex-none rounded-full opacity-40" style={{ backgroundColor: DELAY_COLORS.onTime }} />
                                        Pozycja orientacyjna
                                    </li>
                                </ul>
                            </>
                        )}
                    </div>
                )}

                <p className="mt-2.5 flex items-center gap-1.5 border-t border-white/8 px-1 pt-2 text-[11px] text-slate-500">
                    <span aria-hidden="true" className="h-0.5 w-2.5 flex-none rounded-full bg-slate-500/60" />
                    Linie zagraniczne — tło, bez pociągów
                </p>
            </div>
        </div>
    )
}
