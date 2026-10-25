'use client'
import { useDeferredValue, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import Link from 'next/link'
import { ChevronRight, MapPin, Search } from 'lucide-react'
import { api, type MapStation } from '@/lib/api'
import { Badge, Button, Card, EmptyState, Input, Spinner } from '@/lib/ui'
import { NavShell } from '@/components/NavShell'

const PAGE_SIZE = 100

const collator = new Intl.Collator('pl', { sensitivity: 'base' })

// Lowercase + strip diacritics so "lodz" matches "Łódź".
function normalize(value: string): string {
    return value
        .toLocaleLowerCase('pl')
        .replace(/ł/g, 'l')
        .normalize('NFD')
        .replace(/[̀-ͯ]/g, '')
}

function formatCoord(value: number, positive: string, negative: string): string {
    return `${Math.abs(value).toFixed(4)}° ${value >= 0 ? positive : negative}`
}

function StationRow({ station }: { station: MapStation }) {
    return (
        <li>
            <Link
                href={`/stations/${station.external_id}`}
                className="group block rounded-xl focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-500"
            >
                <Card className="flex items-center gap-3 transition-colors group-hover:border-blue-500/50 group-hover:bg-[#1e2133]">
                    <MapPin size={16} className="flex-shrink-0 text-blue-400" aria-hidden="true" />
                    <div className="min-w-0 flex-1">
                        <p className="truncate text-sm font-semibold text-white">{station.name}</p>
                        <div className="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-slate-500">
                            {station.city && <span className="text-slate-400">{station.city}</span>}
                            <span className="font-mono tabular-nums">
                                {formatCoord(station.latitude, 'N', 'S')}, {formatCoord(station.longitude, 'E', 'W')}
                            </span>
                        </div>
                    </div>
                    <Badge variant="outline" className="hidden flex-shrink-0 font-mono sm:inline-flex">
                        #{station.external_id}
                    </Badge>
                    <ChevronRight
                        size={16}
                        className="flex-shrink-0 text-slate-600 transition-colors group-hover:text-blue-400"
                        aria-hidden="true"
                    />
                </Card>
            </Link>
        </li>
    )
}

export default function StacjePage() {
    const [query, setQuery] = useState('')
    const [visible, setVisible] = useState(PAGE_SIZE)
    const deferredQuery = useDeferredValue(query)

    const stationsQuery = useQuery({
        queryKey: ['mapStations'],
        queryFn: api.getMapStations,
        staleTime: 60 * 60 * 1000,
    })

    const sorted = useMemo(
        () =>
            (stationsQuery.data?.stations ?? [])
                .map((s) => ({ station: s, haystack: normalize(`${s.name} ${s.city ?? ''}`) }))
                .sort((a, b) => collator.compare(a.station.name, b.station.name)),
        [stationsQuery.data],
    )

    const filtered = useMemo(() => {
        const needle = normalize(deferredQuery.trim())
        return needle ? sorted.filter((s) => s.haystack.includes(needle)) : sorted
    }, [sorted, deferredQuery])

    const shown = filtered.slice(0, visible)

    return (
        <NavShell title="Stacje">
            <div className="mx-auto max-w-2xl space-y-4 p-4 md:p-6">
                <div>
                    <h1 className="text-xl font-bold text-white">Stacje</h1>
                    <p className="mt-0.5 text-sm text-slate-500">Lista stacji kolejowych w kolejności alfabetycznej</p>
                </div>

                <div className="relative">
                    <Search
                        size={16}
                        className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-slate-500"
                        aria-hidden="true"
                    />
                    <Input
                        type="search"
                        aria-label="Filtruj stacje po nazwie lub mieście"
                        placeholder="Szukaj po nazwie lub mieście…"
                        value={query}
                        onChange={(e) => {
                            setQuery(e.target.value)
                            setVisible(PAGE_SIZE)
                        }}
                        className="pl-9"
                    />
                </div>

                {stationsQuery.isLoading ? (
                    <div className="flex justify-center py-16">
                        <Spinner className="h-8 w-8" />
                    </div>
                ) : stationsQuery.isError ? (
                    <EmptyState
                        title="Wystąpił błąd"
                        description="Nie udało się pobrać listy stacji."
                        action={
                            <Button variant="secondary" onClick={() => stationsQuery.refetch()}>
                                Spróbuj ponownie
                            </Button>
                        }
                    />
                ) : sorted.length === 0 ? (
                    <EmptyState
                        icon={<MapPin />}
                        title="Brak stacji"
                        description="Nie znaleziono żadnych stacji w bazie danych."
                    />
                ) : filtered.length === 0 ? (
                    <EmptyState
                        icon={<Search />}
                        title="Brak wyników"
                        description={`Żadna stacja nie pasuje do frazy „${deferredQuery.trim()}”.`}
                    />
                ) : (
                    <>
                        <p className="text-xs text-slate-500" aria-live="polite">
                            {filtered.length === sorted.length
                                ? `${sorted.length} stacji`
                                : `${filtered.length} z ${sorted.length} stacji`}
                        </p>
                        <ul className="space-y-2">
                            {shown.map(({ station }) => (
                                <StationRow key={station.external_id} station={station} />
                            ))}
                        </ul>
                        {filtered.length > shown.length && (
                            <div className="flex justify-center pt-2">
                                <Button variant="secondary" onClick={() => setVisible((v) => v + PAGE_SIZE)}>
                                    Pokaż więcej ({filtered.length - shown.length} pozostało)
                                </Button>
                            </div>
                        )}
                    </>
                )}
            </div>
        </NavShell>
    )
}
