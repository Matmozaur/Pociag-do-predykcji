'use client'

import 'leaflet/dist/leaflet.css'
import L from 'leaflet'
import Link from 'next/link'
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowRight, MapPin, TrainFront } from 'lucide-react'
import { CircleMarker, GeoJSON, MapContainer, Pane, Popup, TileLayer, Tooltip, useMap, useMapEvents } from 'react-leaflet'
import { formatDelay, api, type MapStation, type TrainMapPoint, type TrainMapStop } from '@/lib/api'
import { Spinner, cn } from '@/lib/ui'

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
function buildTrainStyle(tone: DelayTone, lowConfidence: boolean, selected: boolean): L.PathOptions {
    return {
        color: selected ? '#f8fafc' : '#0a0c14',
        weight: selected ? 2 : 1.25,
        opacity: lowConfidence ? 0.45 : 1,
        fillColor: DELAY_COLORS[tone],
        fillOpacity: lowConfidence ? 0.4 : 0.95,
    }
}
const TRAIN_STYLES = new Map<string, L.PathOptions>()
function trainStyle(tone: DelayTone, lowConfidence: boolean, selected: boolean): L.PathOptions {
    const key = `${tone}:${lowConfidence}:${selected}`
    let style = TRAIN_STYLES.get(key)
    if (!style) {
        style = buildTrainStyle(tone, lowConfidence, selected)
        TRAIN_STYLES.set(key, style)
    }
    return style
}

function hasCoords(stop?: TrainMapStop): stop is TrainMapStop & { latitude: number; longitude: number } {
    return stop?.latitude != null && stop.longitude != null
}

// Linear interpolation between the segment's stops by wall-clock time, clamped to the
// segment. Falls back to the server position whenever the segment can't be lerped.
function estimatePosition(train: TrainMapPoint, now: number): [number, number] {
    const { previous_stop: prev, next_stop: next } = train
    if (train.phase === 'en_route' && hasCoords(prev) && hasCoords(next)) {
        const start = Date.parse(prev.time)
        const end = Date.parse(next.time)
        if (Number.isFinite(start) && Number.isFinite(end) && end > start) {
            const f = Math.min(1, Math.max(0, (now - start) / (end - start)))
            return [
                prev.latitude + (next.latitude - prev.latitude) * f,
                prev.longitude + (next.longitude - prev.longitude) * f,
            ]
        }
    }
    return [train.latitude, train.longitude]
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
    labelled: boolean
    renderer: L.Renderer
    onSelect: (station: MapStation) => void
}

const StationMarker = memo(function StationMarker({ station, selected, labelled, renderer, onSelect }: StationMarkerProps) {
    const center = useMemo<L.LatLngTuple>(() => [station.latitude, station.longitude], [station.latitude, station.longitude])
    const eventHandlers = useMemo(() => ({ click: () => onSelect(station) }), [onSelect, station])
    return (
        <CircleMarker
            center={center}
            radius={selected ? 10 : 6}
            renderer={renderer}
            pathOptions={selected ? SELECTED_STATION_PATH_OPTIONS : STATION_PATH_OPTIONS}
            eventHandlers={eventHandlers}
        >
            {labelled ? (
                <Tooltip
                    key="label"
                    direction="top"
                    offset={[0, -6]}
                    opacity={1}
                    permanent
                    className="station-label-tooltip"
                >
                    <span className="font-semibold">{station.name}</span>
                </Tooltip>
            ) : (
                <Tooltip key="hover" direction="top" offset={[0, -6]} opacity={1} className="station-label-tooltip">
                    <span className="font-semibold">{station.name}</span>
                </Tooltip>
            )}
        </CircleMarker>
    )
})

interface StationsLayerProps {
    stations: MapStation[]
    selectedStationId?: number
    renderer: L.Renderer
    onStationSelect?: (station: MapStation) => void
}

function StationsLayer({ stations, selectedStationId, renderer, onStationSelect }: StationsLayerProps) {
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

    const showLabels = view.zoom >= STATION_LABEL_MIN_ZOOM
    return (
        <>
            {stations.map((station) => (
                <StationMarker
                    key={station.external_id}
                    station={station}
                    selected={station.external_id === selectedStationId}
                    labelled={showLabels && view.bounds.contains([station.latitude, station.longitude])}
                    renderer={renderer}
                    onSelect={handleSelect}
                />
            ))}
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
// (left on desktop, bottom sheet on mobile) and the layers panel (top right).
function popupPanPadding(): { topLeft: L.PointTuple; bottomRight: L.PointTuple } {
    if (window.innerWidth >= 768) return { topLeft: [430, 24], bottomRight: [260, 24] }
    return { topLeft: [12, 24], bottomRight: [12, Math.round(window.innerHeight * 0.5)] }
}

interface TrainMarkerProps {
    train: TrainMapPoint
    latitude: number
    longitude: number
    dataAsOf?: string
    renderer: L.Renderer
}

const TrainMarker = memo(function TrainMarker({ train, latitude, longitude, dataAsOf, renderer }: TrainMarkerProps) {
    const [open, setOpen] = useState(false)
    const center = useMemo<L.LatLngTuple>(() => [latitude, longitude], [latitude, longitude])
    const padding = useMemo(popupPanPadding, [])
    const eventHandlers = useMemo(
        () => ({ popupopen: () => setOpen(true), popupclose: () => setOpen(false) }),
        [],
    )
    return (
        <CircleMarker
            center={center}
            radius={open ? 7 : 5}
            renderer={renderer}
            pathOptions={trainStyle(delayTone(train.delay_minutes), train.confidence === 'low', open)}
            eventHandlers={eventHandlers}
        >
            <Popup
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

function TrainsLayer({ trains, dataAsOf, now, renderer }: { trains: TrainMapPoint[]; dataAsOf?: string; now: number | null; renderer: L.Renderer }) {
    return (
        <>
            {trains.map((train) => {
                const [latitude, longitude] = now == null ? [train.latitude, train.longitude] : estimatePosition(train, now)
                return (
                    <TrainMarker
                        key={train.operation_id}
                        train={train}
                        latitude={latitude}
                        longitude={longitude}
                        dataAsOf={dataAsOf}
                        renderer={renderer}
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
}

export function TrafficMapClient({ selectedStationId, onStationSelect }: TrafficMapClientProps = {}) {
    const [polandGeo, setPolandGeo] = useState<PolandFeature | null>(null)
    const [showStations, setShowStations] = useState(true)
    const [showTrains, setShowTrains] = useState(true)

    // One canvas for both stations and trains: a Leaflet canvas swallows pointer events
    // for everything underneath it, so two stacked canvases would leave the lower layer
    // unclickable. It lives in trainsPane (above stationsPane); trains are drawn last.
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
    // server positions are shown as-is until the next tick moves them on.
    const [tick, setTick] = useState<number | null>(null)
    useEffect(() => {
        if (!showTrains) return
        const id = window.setInterval(() => setTick(Date.now()), TRAIN_TICK_MS)
        return () => window.clearInterval(id)
    }, [showTrains])
    const now = tick != null && tick > trainsQuery.dataUpdatedAt ? tick : null

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
    const stationsDrawn = showStations && stationsData != null

    const trainCount = trainsData?.trains.length ?? 0
    const unpositioned = trainsData?.unpositioned_count ?? 0

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
                    railway tiles → outside-Poland mask → station dots → trains. */}
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
                            renderer={pointsRenderer}
                            onStationSelect={onStationSelect}
                        />
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
                            renderer={pointsRenderer}
                        />
                    )}
                </Pane>
                <PanToStation station={selectedStation} />
            </MapContainer>

            <div className="absolute right-3 top-3 z-[450] w-56 rounded-xl border border-white/10 bg-[#121622]/90 p-2.5 text-xs text-slate-300 shadow-lg shadow-black/30 backdrop-blur md:right-4 md:top-4">
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
            </div>
        </div>
    )
}
