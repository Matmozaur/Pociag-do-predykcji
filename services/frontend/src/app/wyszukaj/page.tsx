'use client'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { format } from 'date-fns'
import Link from 'next/link'
import { Clock, MapPin, Search, Train } from 'lucide-react'
import { gateway, type ScheduleSearchResult, type StationSuggestion } from '@/lib/api'
import { Button, Card, EmptyState, Input, Spinner } from '@/lib/ui'
import { NavShell } from '@/components/NavShell'

interface SearchParams {
    from: string
    to: string
    date: string
}

const CARRIER_COLORS: Record<string, string> = {
    IC: 'bg-red-500/20 text-red-400',
    TLK: 'bg-orange-500/20 text-orange-400',
    KM: 'bg-purple-500/20 text-purple-400',
    REG: 'bg-slate-500/20 text-slate-400',
}

interface StationAutocompleteProps {
    label: string
    placeholder?: string
    value: string
    onSelect: (station: StationSuggestion) => void
}

function StationAutocomplete({ label, placeholder, value, onSelect }: StationAutocompleteProps) {
    const [inputValue, setInputValue] = useState(value)
    const [debouncedQ, setDebouncedQ] = useState('')
    const [open, setOpen] = useState(false)
    const containerRef = useRef<HTMLDivElement>(null)

    useEffect(() => {
        setInputValue(value)
    }, [value])

    useEffect(() => {
        const timer = setTimeout(() => {
            setDebouncedQ(inputValue)
        }, 300)
        return () => clearTimeout(timer)
    }, [inputValue])

    const stationsQuery = useQuery({
        queryKey: ['stations', debouncedQ],
        queryFn: () => gateway.searchStations(debouncedQ),
        enabled: debouncedQ.length >= 2,
        staleTime: 60_000,
    })

    useEffect(() => {
        function handleOutsideClick(e: MouseEvent) {
            if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
                setOpen(false)
            }
        }

        document.addEventListener('mousedown', handleOutsideClick)
        return () => document.removeEventListener('mousedown', handleOutsideClick)
    }, [])

    const suggestions = useMemo(() => stationsQuery.data?.suggestions ?? [], [stationsQuery.data])

    return (
        <div ref={containerRef} className="relative">
            <div className="relative">
                <Input
                    label={label}
                    placeholder={placeholder ?? 'Wpisz nazwe stacji...'}
                    value={inputValue}
                    onChange={(e) => {
                        setInputValue(e.target.value)
                        setOpen(true)
                    }}
                    onFocus={() => setOpen(true)}
                    autoComplete="off"
                />
                {stationsQuery.isLoading && debouncedQ.length >= 2 && (
                    <div className="absolute right-3 top-1/2 mt-2 -translate-y-1/2">
                        <Spinner className="h-4 w-4" />
                    </div>
                )}
            </div>

            {open && suggestions.length > 0 && (
                <ul className="absolute z-50 mt-1 w-full overflow-hidden rounded-lg border border-[#2d3148] bg-[#222536] shadow-xl">
                    {suggestions.map((station) => (
                        <li key={station.external_id}>
                            <button
                                className="w-full px-3 py-2.5 text-left text-sm transition-colors hover:bg-[#2d3148]"
                                onMouseDown={(e) => {
                                    e.preventDefault()
                                }}
                                onClick={() => {
                                    setInputValue(station.name)
                                    setOpen(false)
                                    onSelect(station)
                                }}
                            >
                                <div className="flex items-center gap-2">
                                    <MapPin size={14} className="flex-shrink-0 text-slate-500" />
                                    <div>
                                        <span className="text-white">{station.name}</span>
                                        {station.city && station.city !== station.name && (
                                            <span className="ml-1 text-xs text-slate-500">({station.city})</span>
                                        )}
                                    </div>
                                </div>
                            </button>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    )
}

function ConnectionResultCard({ result }: { result: ScheduleSearchResult }) {
    const category = result.commercial_category ?? result.carrier.code
    const colorClass = CARRIER_COLORS[category] ?? 'bg-blue-500/20 text-blue-400'

    return (
        <Link href={`/rozklad/${result.route_id}`} className="block">
            <Card className="flex items-center gap-4">
                <span className={`flex-shrink-0 rounded-md px-2 py-1 text-xs font-bold ${colorClass}`}>
                    {category}
                </span>

                <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-semibold text-white">{result.train_name}</p>
                    <div className="mt-0.5 flex items-center gap-1 text-xs text-slate-500">
                        <MapPin size={11} />
                        <span>{result.carrier.name}</span>
                        {result.stops_count != null && <span className="ml-1">· {result.stops_count} przystankow</span>}
                    </div>
                </div>

                <div className="flex flex-shrink-0 flex-col items-end text-sm">
                    <div className="flex items-center gap-2">
                        <span className="font-semibold text-white">{result.departure.time}</span>
                        <span className="text-slate-600">→</span>
                        <span className="font-semibold text-white">{result.arrival.time}</span>
                    </div>
                    {result.duration_minutes != null && (
                        <div className="mt-0.5 flex items-center gap-1 text-xs text-slate-500">
                            <Clock size={11} />
                            <span>
                                {Math.floor(result.duration_minutes / 60)}h{' '}
                                {result.duration_minutes % 60 > 0 ? `${result.duration_minutes % 60}min` : ''}
                            </span>
                        </div>
                    )}
                </div>
            </Card>
        </Link>
    )
}

export default function WyszukajPage() {
    const [fromStation, setFromStation] = useState<StationSuggestion | null>(null)
    const [toStation, setToStation] = useState<StationSuggestion | null>(null)
    const [date, setDate] = useState(format(new Date(), 'yyyy-MM-dd'))
    const [submitted, setSubmitted] = useState<SearchParams | null>(null)

    const scheduleQuery = useQuery({
        queryKey: ['schedules', submitted],
        queryFn: () => gateway.searchSchedules(submitted!),
        enabled:
            submitted !== null &&
            submitted.from.length > 0 &&
            submitted.to.length > 0 &&
            submitted.date.length > 0,
        staleTime: 30_000,
    })

    function handleSearch() {
        if (!fromStation || !toStation) return
        setSubmitted({
            from: String(fromStation.external_id),
            to: String(toStation.external_id),
            date,
        })
    }

    return (
        <NavShell title="Wyszukaj połączenie">
            <div className="max-w-2xl mx-auto p-4 md:p-6 space-y-6">
                <div>
                    <h1 className="text-xl font-bold text-white mb-1">Wyszukaj połączenie</h1>
                    <p className="text-sm text-slate-500">Znajdź połączenie kolejowe między stacjami</p>
                </div>

                {/* Search form */}
                <div className="bg-[#1a1d27] border border-[#2d3148] rounded-xl p-4 space-y-4">
                    <StationAutocomplete
                        label="Skąd"
                        placeholder="Stacja odjazdu..."
                        value={fromStation?.name ?? ''}
                        onSelect={setFromStation}
                    />
                    <StationAutocomplete
                        label="Dokąd"
                        placeholder="Stacja docelowa..."
                        value={toStation?.name ?? ''}
                        onSelect={setToStation}
                    />
                    <div className="flex flex-col gap-1">
                        <label className="text-xs font-medium text-slate-400 uppercase tracking-wide">
                            Data podróży
                        </label>
                        <input
                            type="date"
                            value={date}
                            min={format(new Date(), 'yyyy-MM-dd')}
                            onChange={(e) => setDate(e.target.value)}
                            className="w-full rounded-lg bg-[#222536] border border-[#2d3148] px-3 py-2 text-slate-100 focus:outline-none focus:border-blue-500 focus:ring-1 focus:ring-blue-500 transition-colors"
                        />
                    </div>
                    <Button
                        className="w-full"
                        size="lg"
                        onClick={handleSearch}
                        disabled={!fromStation || !toStation || scheduleQuery.isLoading}
                    >
                        {scheduleQuery.isLoading ? (
                            <Spinner className="mr-2 h-4 w-4" />
                        ) : (
                            <Search size={16} className="mr-2" />
                        )}
                        Szukaj połączeń
                    </Button>
                </div>

                {/* Results */}
                {submitted && (
                    <div>
                        {scheduleQuery.isLoading ? (
                            <div className="flex justify-center py-12">
                                <Spinner className="h-8 w-8" />
                            </div>
                        ) : scheduleQuery.isError ? (
                            <EmptyState
                                title="Wystąpił błąd"
                                description="Nie udało się pobrać wyników. Spróbuj ponownie."
                                action={
                                    <Button variant="secondary" onClick={() => scheduleQuery.refetch()}>
                                        Spróbuj ponownie
                                    </Button>
                                }
                            />
                        ) : scheduleQuery.data && scheduleQuery.data.data.length > 0 ? (
                            <div className="space-y-3">
                                <p className="text-xs text-slate-500">
                                    Znaleziono {scheduleQuery.data.pagination.total} połączeń
                                </p>
                                {scheduleQuery.data.data.map((result) => (
                                    <ConnectionResultCard key={result.route_id} result={result} />
                                ))}
                            </div>
                        ) : (
                            <EmptyState
                                icon={<Train />}
                                title="Brak połączeń"
                                description="Nie znaleziono połączeń dla wybranych stacji i daty."
                            />
                        )}
                    </div>
                )}
            </div>
        </NavShell>
    )
}
