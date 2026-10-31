'use client'
import { useId, useState, type Ref } from 'react'
import Link from 'next/link'
import { useQuery } from '@tanstack/react-query'
import { format } from 'date-fns'
import { Route } from 'lucide-react'
import { api, type ScheduleSearchResult } from '@/lib/api'
import { Spinner } from '@/lib/ui'

// Same palette as the /wyszukaj result cards.
const CARRIER_COLORS: Record<string, string> = {
    IC: 'bg-red-500/20 text-red-400',
    TLK: 'bg-orange-500/20 text-orange-400',
    KM: 'bg-purple-500/20 text-purple-400',
    REG: 'bg-slate-500/20 text-slate-400',
}

// The api's maximum page; a day's connections between two stations fit in it.
const RESULTS_LIMIT = 100

function formatDuration(minutes: number): string {
    const h = Math.floor(minutes / 60)
    const m = minutes % 60
    return h > 0 ? `${h}h${m > 0 ? ` ${m}min` : ''}` : `${m}min`
}

function ConnectionRow({ result }: { result: ScheduleSearchResult }) {
    const category = result.commercial_category ?? result.carrier.code
    const chipClass = CARRIER_COLORS[category] ?? 'bg-blue-500/20 text-blue-400'
    const meta = result.stops_count != null ? `${result.carrier.name} · ${result.stops_count} przystanków` : result.carrier.name

    return (
        <Link href={`/rozklad/${result.route_id}`} className="group flex items-center gap-3 rounded-lg px-2 py-2.5 transition-colors hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400">
            <span className={`w-11 flex-none truncate rounded-md px-1.5 py-1 text-center text-[11px] font-bold ${chipClass}`}>{category}</span>
            <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium text-slate-100">{result.train_name}</span>
                <span className="block truncate text-xs text-slate-500" title={meta}>{meta}</span>
            </span>
            <span className="flex flex-none flex-col items-end">
                <span className="whitespace-nowrap text-sm font-semibold tabular-nums text-white">
                    {result.departure.time} <span className="font-normal text-slate-600">→</span> {result.arrival.time}
                </span>
                {result.duration_minutes != null && (
                    <span className="text-[11px] tabular-nums text-slate-500">{formatDuration(result.duration_minutes)}</span>
                )}
            </span>
        </Link>
    )
}

export interface ConnectionResultsProps {
    /** Station id or city / station-name prefix, passed to `/schedules/search` as is. */
    from: string
    to: string
    headingRef?: Ref<HTMLHeadingElement>
}

export function ConnectionResults({ from, to, headingRef }: ConnectionResultsProps) {
    const uid = useId()
    const titleId = `${uid}-title`
    const dateId = `${uid}-date`
    const today = format(new Date(), 'yyyy-MM-dd')
    const [date, setDate] = useState(today)

    const params = { from, to, date, limit: RESULTS_LIMIT }
    const scheduleQuery = useQuery({
        queryKey: ['schedules', params],
        queryFn: () => api.searchSchedules(params),
        enabled: date.length > 0,
        staleTime: 30_000,
    })

    // The heading names the pair even while loading or with no results: a station id is
    // looked up in the map's station cache, a city / name prefix is shown as typed.
    const { data: stationsData } = useQuery({
        queryKey: ['mapStations'],
        queryFn: api.getMapStations,
        staleTime: 60 * 60 * 1000,
    })
    const endName = (end: string) =>
        /^\d+$/.test(end) ? stationsData?.stations.find((s) => s.external_id === Number(end))?.name : end
    const fromName = endName(from)
    const toName = endName(to)

    const results = scheduleQuery.data?.data ?? []
    const title = fromName && toName ? `${fromName} → ${toName}` : 'Połączenia'

    return (
        <section aria-labelledby={titleId}>
            <header className="mb-4">
                <div className="mb-1.5 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-blue-300">
                    <Route size={14} aria-hidden="true" /> Połączenia
                </div>
                <h2
                    id={titleId}
                    ref={headingRef}
                    tabIndex={-1}
                    className="text-xl font-semibold tracking-tight text-white focus:outline-none"
                >
                    {title}
                </h2>
                <div className="mt-3 flex items-center gap-2">
                    <label htmlFor={dateId} className="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-500">
                        Data
                    </label>
                    <input
                        id={dateId}
                        type="date"
                        value={date}
                        min={today}
                        onChange={(e) => setDate(e.target.value)}
                        className="rounded-lg border border-[#2d3148] bg-[#222536] px-2 py-1 text-sm text-slate-100 transition-colors focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500"
                    />
                </div>
            </header>

            {date.length === 0 ? (
                <p className="py-4 text-sm text-slate-400">Wybierz datę podróży.</p>
            ) : scheduleQuery.isLoading ? (
                <div className="flex justify-center py-8">
                    <Spinner />
                </div>
            ) : scheduleQuery.isError ? (
                <div className="rounded-xl border border-red-500/25 bg-red-500/10 p-3 text-sm text-slate-200" role="alert">
                    <p>Nie udało się pobrać połączeń.</p>
                    <button
                        onClick={() => void scheduleQuery.refetch()}
                        className="mt-2 text-xs font-semibold text-red-300 underline underline-offset-4 hover:text-white"
                    >
                        Spróbuj ponownie
                    </button>
                </div>
            ) : results.length > 0 ? (
                <div>
                    <p className="mb-2 text-xs text-slate-500">
                        Znaleziono {scheduleQuery.data?.pagination.total ?? results.length} połączeń
                    </p>
                    <div>
                        {results.map((result) => (
                            <ConnectionRow key={result.route_id} result={result} />
                        ))}
                    </div>
                </div>
            ) : (
                <div className="flex items-center gap-2 py-4 text-sm text-slate-400">
                    <Route size={17} className="text-slate-500" aria-hidden="true" /> Brak połączeń dla wybranych stacji i daty.
                </div>
            )}
        </section>
    )
}
