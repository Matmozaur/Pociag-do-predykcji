const BASE =
    typeof window !== 'undefined'
        ? (process.env.NEXT_PUBLIC_API_BASE ?? '/bff')
        : `${process.env.API_URL ?? 'http://localhost:8080'}`

/** Error thrown by `apiFetch` for non-2xx responses; `status` is the HTTP status code. */
export class ApiError extends Error {
    constructor(
        message: string,
        readonly status: number,
    ) {
        super(message)
        this.name = 'ApiError'
    }
}

export async function apiFetch<T>(
    path: string,
    params?: Record<string, string | undefined>,
): Promise<T> {
    const base =
        typeof window !== 'undefined' ? window.location.origin : 'http://localhost:3000'
    const url = new URL(`${BASE}${path}`, base)
    if (params) {
        Object.entries(params).forEach(([k, v]) => {
            if (v !== undefined) {
                url.searchParams.set(k, v)
            }
        })
    }

    const res = await fetch(url.toString(), {
        headers: { 'Content-Type': 'application/json' },
        next: { revalidate: 30 },
    })

    if (!res.ok) {
        const err = await res.json().catch(() => ({ message: res.statusText }))
        throw new ApiError((err as { message?: string }).message ?? 'Błąd API', res.status)
    }

    return res.json() as Promise<T>
}

export interface StationSuggestion {
    external_id: number
    name: string
    city?: string
}

export interface StationSuggestionsResponse {
    suggestions: StationSuggestion[]
}

export interface MapStation {
    external_id: number
    name: string
    city?: string
    latitude: number
    longitude: number
}

export interface StationMapResponse {
    stations: MapStation[]
}

export interface TrainMapStop {
    station_name?: string
    /** RFC 3339 instant: effective departure for previous_stop, effective arrival for next_stop. */
    time: string
    latitude?: number
    longitude?: number
}

export interface TrainMapPoint {
    operation_id: number
    train_name: string
    carrier_code?: string
    status: 'not_started' | 'in_progress' | 'completed' | 'cancelled' | 'partial_cancelled'
    phase: 'not_departed' | 'at_station' | 'en_route' | 'arrived'
    delay_minutes?: number
    latitude: number
    longitude: number
    progress: number
    method: 'station' | 'interpolated' | 'interpolated_sparse'
    confidence: 'high' | 'medium' | 'low'
    previous_stop?: TrainMapStop
    next_stop?: TrainMapStop
    origin?: string
    destination?: string
}

export interface TrainMapResponse {
    trains: TrainMapPoint[]
    unpositioned_count: number
    generated_at: string
    data_as_of?: string
}

export interface StationBoardRow {
    operation_id: number
    train_name: string
    train_number?: string
    commercial_category?: string
    carrier?: {
        code: string
        name?: string
    }
    origin?: string
    destination?: string
    planned_time?: string
    expected_time?: string
    expected_at?: string
    delay_minutes?: number
    platform?: string
    track?: string
    status: 'not_started' | 'in_progress' | 'completed' | 'cancelled' | 'partial_cancelled'
    is_cancelled: boolean
    is_confirmed: boolean
}

export interface StationBoardView {
    station: {
        external_id: number
        name: string
        city?: string
        latitude?: number
        longitude?: number
    }
    generated_at: string
    data_as_of?: string
    at_station: StationBoardRow[]
    arrivals: StationBoardRow[]
    departures: StationBoardRow[]
}

export interface CarrierInfo {
    code: string
    name?: string
}

export interface ScheduleSearchResult {
    route_id: number
    train_name: string
    carrier: CarrierInfo
    commercial_category?: string
    departure: { station_name: string; station_external_id?: number; time: string }
    arrival: { station_name: string; station_external_id?: number; time: string }
    duration_minutes?: number
    stops_count?: number
}

export interface PaginationMeta {
    total: number
    limit: number
    offset: number
    has_more: boolean
}

export interface ScheduleSearchResponse {
    data: ScheduleSearchResult[]
    pagination: PaginationMeta
    query: { from?: string; to?: string; date?: string }
}

export interface ScheduleStopView {
    station_name: string
    station_external_id?: number
    order: number
    arrival_time?: string
    departure_time?: string
    platform?: string
}

export interface ScheduleDetailView {
    route_id: number
    train_name: string
    carrier: CarrierInfo
    commercial_category?: string
    national_number?: string
    stops: ScheduleStopView[]
    operating_dates: string[]
    total_duration_minutes?: number
}

export interface LiveTrainSummary {
    operation_id: number
    train_name: string
    carrier_code?: string
    status: string
    status_code?: string
    current_station?: string
    next_station?: string
    delay_minutes?: number
    origin?: string
    destination?: string
}

export interface LiveTrainsResponse {
    data: LiveTrainSummary[]
    pagination: PaginationMeta
    generated_at: string
}

export interface TrainStopView {
    station_name: string
    station_external_id?: number
    sequence: number
    planned_arrival?: string
    planned_departure?: string
    actual_arrival?: string
    actual_departure?: string
    arrival_delay_minutes?: number
    departure_delay_minutes?: number
    is_confirmed: boolean
    is_cancelled: boolean
}

export interface TrainDetailView {
    operation_id: number
    train_name: string
    carrier?: {
        code?: string
        name?: string
    }
    operating_date: string
    status: string
    status_code?: string
    stops: TrainStopView[]
}

export interface DisruptionSummaryView {
    id: number
    start_station?: string
    end_station?: string
    message: string
    date_from?: string
    date_to?: string
    affected_routes_count?: number
    severity?: 'low' | 'medium' | 'high'
}

export interface DisruptionListView {
    data: DisruptionSummaryView[]
    pagination: PaginationMeta
}

export interface DashboardOverview {
    statistics: {
        date: string
        total_trains: number
        in_progress?: number
        completed?: number
        cancelled?: number
        avg_delay_minutes?: number
        on_time_percentage?: number
    }
    disruptions_active: number
    data_freshness: {
        schedules_last_updated?: string
        operations_last_updated?: string
    }
}

export const api = {
    searchStations: (q: string, limit = 10) =>
        apiFetch<StationSuggestionsResponse>('/api/v1/search/stations', {
            q,
            limit: String(limit),
        }),

    searchSchedules: (params: {
        from: string
        to: string
        date: string
        carriers?: string
        categories?: string
        sort?: 'departure' | 'arrival' | 'duration'
        limit?: number
        offset?: number
    }) =>
        apiFetch<ScheduleSearchResponse>('/api/v1/schedules/search', {
            from: params.from,
            to: params.to,
            date: params.date,
            carriers: params.carriers,
            categories: params.categories,
            sort: params.sort,
            limit: params.limit ? String(params.limit) : undefined,
            offset: params.offset !== undefined ? String(params.offset) : undefined,
        }),

    getScheduleDetail: (routeId: number) =>
        apiFetch<ScheduleDetailView>(`/api/v1/schedules/${routeId}`),

    getLiveTrains: (params?: { carriers?: string; limit?: number; offset?: number }) =>
        apiFetch<LiveTrainsResponse>('/api/v1/trains/live', {
            carriers: params?.carriers,
            limit: params?.limit ? String(params.limit) : undefined,
            offset: params?.offset ? String(params.offset) : undefined,
        }),

    // Every active train: the api orders by operation id and caps a page at 100, so a
    // client-side sort (e.g. by delay) needs all pages, not just the first one.
    getAllLiveTrains: async (params?: { carriers?: string }): Promise<LiveTrainsResponse> => {
        const pageSize = 100
        const first = await api.getLiveTrains({ ...params, limit: pageSize })
        const offsets: number[] = []
        for (let offset = pageSize; offset < first.pagination.total; offset += pageSize) offsets.push(offset)
        const rest = await Promise.all(offsets.map((offset) => api.getLiveTrains({ ...params, limit: pageSize, offset })))
        const data = [first, ...rest].flatMap((page) => page.data)
        // Pages are separate queries, so a train can shift across a page boundary; keep one copy.
        const unique = [...new Map(data.map((train) => [train.operation_id, train])).values()]
        return { ...first, data: unique, pagination: { ...first.pagination, limit: unique.length } }
    },

    getTrainDetail: (operationId: number) =>
        apiFetch<TrainDetailView>(`/api/v1/trains/${operationId}`),

    listDisruptions: (limit = 20, offset?: number) =>
        apiFetch<DisruptionListView>('/api/v1/disruptions', {
            limit: String(limit),
            offset: offset !== undefined ? String(offset) : undefined,
        }),

    getDashboardOverview: () => apiFetch<DashboardOverview>('/api/v1/dashboard/overview'),

    getMapStations: () => apiFetch<StationMapResponse>('/api/v1/map/stations'),

    getMapTrains: (carriers?: string) =>
        apiFetch<TrainMapResponse>('/api/v1/map/trains', { carriers }),

    getStationBoard: (externalId: number, limit?: number) =>
        apiFetch<StationBoardView>(`/api/v1/stations/${externalId}/board`, {
            limit: limit !== undefined ? String(limit) : undefined,
        }),
}

export function formatDelay(minutes: number | null | undefined): string {
    if (minutes == null) return '-'
    if (minutes === 0) return 'Na czas'
    if (minutes > 0) return `+${minutes} min`
    return `${minutes} min`
}

/** Comparator: largest delay first, unknown delay last, ties by train name. */
export function compareByDelay(
    a: Pick<LiveTrainSummary, 'delay_minutes' | 'train_name'>,
    b: Pick<LiveTrainSummary, 'delay_minutes' | 'train_name'>,
): number {
    const aKnown = a.delay_minutes != null
    const bKnown = b.delay_minutes != null
    if (aKnown !== bKnown) return aKnown ? -1 : 1
    if (aKnown && bKnown && a.delay_minutes !== b.delay_minutes) return b.delay_minutes! - a.delay_minutes!
    return a.train_name.localeCompare(b.train_name, 'pl')
}

/** Returns a new array sorted with `compareByDelay`; the input is not mutated. */
export function sortByDelay<T extends Pick<LiveTrainSummary, 'delay_minutes' | 'train_name'>>(trains: readonly T[]): T[] {
    return [...trains].sort(compareByDelay)
}

export function delayVariant(
    minutes: number | null | undefined,
): 'success' | 'warning' | 'danger' {
    if (minutes == null || minutes <= 1) return 'success'
    if (minutes < 15) return 'warning'
    return 'danger'
}

export function statusLabel(status: string): string {
    const map: Record<string, string> = {
        not_started: 'Nie rozpoczęty',
        in_progress: 'W trasie',
        completed: 'Zakończony',
        cancelled: 'Odwołany',
        partial_cancelled: 'Częściowo odwołany',
    }
    return map[status] ?? status
}

export function statusVariant(status: string): 'success' | 'warning' | 'danger' | 'default' {
    if (status === 'in_progress') return 'success'
    if (status === 'not_started') return 'default'
    if (status === 'cancelled') return 'danger'
    if (status === 'partial_cancelled') return 'warning'
    return 'default'
}
