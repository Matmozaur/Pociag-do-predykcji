'use client'

import Link from 'next/link'
import { useEffect, useId, useRef, useState, type KeyboardEvent, type ReactNode, type Ref } from 'react'
import { ArrowDownLeft, ArrowUpRight, ChevronDown, Clock, MapPinOff, RefreshCw, TrainFront } from 'lucide-react'
import { DataFreshness } from '@/components/DataFreshness'
import { ApiError, delayVariant, formatDelay, type StationBoardRow } from '@/lib/api'
import { useStationBoard } from '@/lib/useStationBoard'
import { Badge, Button, cn } from '@/lib/ui'

const LIMIT_STEPS = [10, 20, 50] as const
const COUNTDOWN_TICK_MS = 30_000

type SectionKey = 'at_station' | 'arrivals' | 'departures'

const SECTIONS: { key: SectionKey; label: string; empty: string; icon: typeof Clock }[] = [
    { key: 'at_station', label: 'Na stacji', empty: 'Brak pociągów na stacji', icon: TrainFront },
    { key: 'arrivals', label: 'Przyjazdy', empty: 'Brak przyjazdów w najbliższych 12 h', icon: ArrowDownLeft },
    { key: 'departures', label: 'Odjazdy', empty: 'Brak odjazdów w najbliższych 12 h', icon: ArrowUpRight },
]

/** Wall clock that re-renders its consumer every `intervalMs`, for relative "za N min" labels. */
function useNow(intervalMs: number) {
    const [now, setNow] = useState(() => Date.now())
    useEffect(() => {
        const timer = setInterval(() => setNow(Date.now()), intervalMs)
        return () => clearInterval(timer)
    }, [intervalMs])
    return now
}

function countdownLabel(expectedAt: string | undefined, now: number): string | null {
    if (!expectedAt) return null
    const at = new Date(expectedAt).getTime()
    if (Number.isNaN(at)) return null
    const minutes = Math.ceil((at - now) / 60_000)
    if (minutes <= 0) return 'teraz'
    if (minutes < 60) return `za ${minutes} min`
    const hours = Math.floor(minutes / 60)
    const rest = minutes % 60
    return rest ? `za ${hours} h ${rest} min` : `za ${hours} h`
}

function platformLabel(row: StationBoardRow): string | null {
    if (row.platform && row.track) return `peron ${row.platform}, tor ${row.track}`
    if (row.platform) return `peron ${row.platform}`
    if (row.track) return `tor ${row.track}`
    return null
}

function BoardRow({ row, section, now }: { row: StationBoardRow; section: SectionKey; now: number }) {
    const delayed = !row.is_cancelled && row.expected_time != null && row.expected_time !== row.planned_time
    const relation = section === 'arrivals' ? (row.origin ? `z ${row.origin}` : null) : row.destination ? `do ${row.destination}` : null
    const platform = platformLabel(row)
    const countdown = row.is_cancelled ? null : countdownLabel(row.expected_at, now)
    const showNumber = row.train_number && !row.train_name.includes(row.train_number)

    return (
        <li>
            <Link
                href={`/pociagi/${row.operation_id}`}
                className={cn(
                    'group flex items-start gap-3 rounded-lg px-2 py-2.5 transition-colors hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400',
                    row.is_cancelled && 'opacity-60',
                )}
            >
                <span className="w-12 flex-none pt-px text-right tabular-nums">
                    {delayed ? (
                        <>
                            <span className="block text-sm font-semibold text-amber-300">{row.expected_time}</span>
                            <span className="block text-[11px] text-slate-500 line-through" aria-label={`planowo ${row.planned_time}`}>{row.planned_time}</span>
                        </>
                    ) : (
                        <span className={cn('block text-sm font-semibold', row.is_cancelled ? 'text-slate-500 line-through' : 'text-white')}>
                            {row.planned_time ?? '—'}
                        </span>
                    )}
                </span>

                <span className="min-w-0 flex-1">
                    <span className="flex min-w-0 items-center gap-1.5">
                        <span className={cn('truncate text-sm font-medium', row.is_cancelled ? 'text-slate-400 line-through' : 'text-slate-100')}>
                            {row.train_name}
                        </span>
                        {showNumber && <span className="flex-none text-xs tabular-nums text-slate-500">{row.train_number}</span>}
                        {row.carrier && (
                            <Badge variant="outline" className="flex-none px-1.5 py-0 text-[10px]">
                                <span title={row.carrier.name}>{row.carrier.code}</span>
                            </Badge>
                        )}
                    </span>
                    {relation && <span className="block truncate text-xs text-slate-400">{relation}</span>}
                    {platform && <span className="block truncate text-[11px] text-slate-500">{platform}</span>}
                </span>

                <span className="flex flex-none flex-col items-end gap-1">
                    {row.is_cancelled ? (
                        <Badge variant="danger">Odwołany</Badge>
                    ) : row.delay_minutes != null ? (
                        <Badge variant={delayVariant(row.delay_minutes)}>{formatDelay(row.delay_minutes)}</Badge>
                    ) : null}
                    {countdown && <span className="text-[11px] tabular-nums text-slate-400">{countdown}</span>}
                </span>
            </Link>
        </li>
    )
}

function SkeletonRows({ count = 4 }: { count?: number }) {
    return (
        <ul aria-hidden="true" className="space-y-1 py-1">
            {Array.from({ length: count }, (_, i) => (
                <li key={i} className="flex items-start gap-3 px-2 py-2.5">
                    <span className="h-4 w-12 flex-none animate-pulse rounded bg-white/5" />
                    <span className="flex-1 space-y-1.5">
                        <span className="block h-4 w-3/5 animate-pulse rounded bg-white/5" />
                        <span className="block h-3 w-2/5 animate-pulse rounded bg-white/5" />
                    </span>
                    <span className="h-4 w-12 flex-none animate-pulse rounded-full bg-white/5" />
                </li>
            ))}
        </ul>
    )
}

function QueryMessage({ children, onRetry }: { children: ReactNode; onRetry: () => void }) {
    return (
        <div className="rounded-xl border border-red-500/25 bg-red-500/10 p-3 text-sm text-slate-200" role="alert">
            <p>{children}</p>
            <button onClick={onRetry} className="mt-2 text-xs font-semibold text-red-300 underline underline-offset-4 hover:text-white">
                Spróbuj ponownie
            </button>
        </div>
    )
}

interface StationBoardProps {
    stationId: number | null
    variant: 'panel' | 'page'
    /** Receives the heading element so the caller can move focus to it. */
    headingRef?: Ref<HTMLHeadingElement>
    /** Shown after the header, e.g. a link to the full board. */
    actions?: ReactNode
}

export function StationBoard({ stationId, variant, headingRef, actions }: StationBoardProps) {
    const uid = useId()
    const titleId = `${uid}-title`
    const [limitState, setLimitState] = useState({ id: stationId, limit: LIMIT_STEPS[0] as number })
    const limit = limitState.id === stationId ? limitState.limit : LIMIT_STEPS[0]
    const [activeTab, setActiveTab] = useState<SectionKey>('departures')
    const tabRefs = useRef<Record<SectionKey, HTMLButtonElement | null>>({ at_station: null, arrivals: null, departures: null })
    const now = useNow(COUNTDOWN_TICK_MS)

    const boardQuery = useStationBoard(stationId, limit)
    const board = boardQuery.data
    const notFound = stationId == null || (boardQuery.error instanceof ApiError && (boardQuery.error.status === 404 || boardQuery.error.status === 400))
    const isPage = variant === 'page'
    const nextLimit = LIMIT_STEPS.find((step) => step > limit)
    const loadingMore = boardQuery.isPlaceholderData && boardQuery.isFetching

    function onTabKeyDown(e: KeyboardEvent<HTMLButtonElement>, index: number) {
        const delta = e.key === 'ArrowRight' ? 1 : e.key === 'ArrowLeft' ? -1 : 0
        if (!delta) return
        e.preventDefault()
        const next = SECTIONS[(index + delta + SECTIONS.length) % SECTIONS.length].key
        setActiveTab(next)
        tabRefs.current[next]?.focus()
    }

    const title = notFound ? 'Nie znaleziono stacji' : (board?.station.name ?? (boardQuery.isError ? `Stacja ${stationId}` : undefined))
    const city = board?.station.city && board.station.city !== board.station.name ? board.station.city : null

    return (
        <section aria-labelledby={titleId} className={cn(isPage && 'space-y-4')}>
            <header className={cn(isPage ? 'rounded-xl border border-[#2d3148] bg-[#1a1d27] p-4' : 'mb-4')}>
                <div className="mb-1.5 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-blue-300">
                    <Clock size={14} aria-hidden="true" /> Tablica stacyjna
                </div>
                <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                        <h1
                            id={titleId}
                            ref={headingRef}
                            tabIndex={-1}
                            className={cn('font-semibold tracking-tight text-white focus:outline-none', isPage ? 'text-2xl' : 'text-xl')}
                        >
                            {title ?? (
                                <>
                                    <span className="sr-only">Ładowanie tablicy stacyjnej</span>
                                    <span aria-hidden="true" className="inline-block h-6 w-48 animate-pulse rounded bg-white/5 align-middle" />
                                </>
                            )}
                        </h1>
                        {city && <p className="mt-0.5 text-sm text-slate-400">{city}</p>}
                    </div>
                    {!notFound && (
                        <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => void boardQuery.refetch()}
                            disabled={boardQuery.isFetching}
                            aria-label={boardQuery.isFetching ? 'Odświeżanie tablicy' : 'Odśwież tablicę'}
                            title={boardQuery.isFetching ? 'Odświeżanie tablicy' : 'Odśwież tablicę'}
                            className="h-9 w-9 flex-none !px-0"
                        >
                            <RefreshCw size={16} className={boardQuery.isFetching ? 'animate-spin' : ''} />
                        </Button>
                    )}
                </div>
                {board && <DataFreshness lastUpdated={board.data_as_of} className="mt-2" />}
                {actions && !notFound && <div className="mt-3">{actions}</div>}
            </header>

            {notFound ? (
                <div className={cn('flex flex-col items-center gap-2 py-10 text-center', isPage && 'rounded-xl border border-[#2d3148] bg-[#1a1d27]')}>
                    <MapPinOff size={28} className="text-slate-500" aria-hidden="true" />
                    <p className="text-sm text-slate-300">Ta stacja nie istnieje lub nie ma jej w naszych danych.</p>
                    <Link href="/wyszukaj" className="text-xs font-medium text-blue-300 hover:text-blue-200">Wyszukaj stację</Link>
                </div>
            ) : boardQuery.isError && !board ? (
                <QueryMessage onRetry={() => void boardQuery.refetch()}>Nie udało się pobrać tablicy stacyjnej.</QueryMessage>
            ) : (
                <>
                    <div role="tablist" aria-label="Sekcje tablicy" className="mb-3 grid grid-cols-3 gap-1 rounded-xl border border-white/8 bg-black/15 p-1 md:hidden">
                        {SECTIONS.map(({ key, label }, index) => {
                            const selected = activeTab === key
                            const count = board?.[key].length
                            return (
                                <button
                                    key={key}
                                    ref={(el) => {
                                        tabRefs.current[key] = el
                                    }}
                                    id={`${uid}-tab-${key}`}
                                    role="tab"
                                    type="button"
                                    aria-selected={selected}
                                    aria-controls={`${uid}-panel-${key}`}
                                    tabIndex={selected ? 0 : -1}
                                    onClick={() => setActiveTab(key)}
                                    onKeyDown={(e) => onTabKeyDown(e, index)}
                                    className={cn(
                                        'rounded-lg px-2 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400',
                                        selected ? 'bg-blue-600/25 text-blue-200' : 'text-slate-400 hover:text-white',
                                    )}
                                >
                                    {label}
                                    {count != null && <span className="ml-1 tabular-nums text-slate-500">{count}</span>}
                                </button>
                            )
                        })}
                    </div>

                    {boardQuery.isError && board && (
                        <QueryMessage onRetry={() => void boardQuery.refetch()}>Nie udało się odświeżyć tablicy. Pokazujemy ostatnie dane.</QueryMessage>
                    )}

                    {SECTIONS.map(({ key, label, empty, icon: Icon }, index) => {
                        const rows = board?.[key] ?? []
                        const headingId = `${uid}-heading-${key}`
                        return (
                            <section
                                key={key}
                                id={`${uid}-panel-${key}`}
                                aria-labelledby={headingId}
                                className={cn(
                                    activeTab === key ? 'block' : 'hidden md:block',
                                    isPage
                                        ? 'rounded-xl border border-[#2d3148] bg-[#1a1d27] p-3 md:p-4'
                                        : cn('md:border-t md:border-white/8 md:pt-4', index > 0 && 'md:mt-3'),
                                )}
                            >
                                <h2 id={headingId} className="mb-1 flex items-center gap-2 px-2 text-sm font-semibold text-white max-md:sr-only">
                                    <Icon size={15} className="text-blue-300" aria-hidden="true" />
                                    {label}
                                    {board && <span className="text-xs font-normal tabular-nums text-slate-500">{rows.length}</span>}
                                </h2>
                                {boardQuery.isLoading ? (
                                    <SkeletonRows />
                                ) : rows.length ? (
                                    <ul className="divide-y divide-white/5">
                                        {rows.map((row) => (
                                            <BoardRow key={row.operation_id} row={row} section={key} now={now} />
                                        ))}
                                    </ul>
                                ) : (
                                    <p className="flex items-center gap-2 px-2 py-4 text-sm text-slate-400">
                                        <Icon size={16} className="text-slate-500" aria-hidden="true" /> {empty}
                                    </p>
                                )}
                                {nextLimit && rows.length >= limit && (
                                    <button
                                        type="button"
                                        onClick={() => setLimitState({ id: stationId, limit: nextLimit })}
                                        disabled={loadingMore}
                                        className="mt-1 flex w-full items-center justify-center gap-1.5 rounded-lg py-2 text-xs font-semibold text-blue-300 transition-colors hover:bg-white/5 hover:text-blue-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400 disabled:opacity-60"
                                    >
                                        {loadingMore ? <RefreshCw size={13} className="animate-spin" aria-hidden="true" /> : <ChevronDown size={14} aria-hidden="true" />}
                                        {loadingMore ? 'Ładowanie…' : 'Pokaż więcej'}
                                    </button>
                                )}
                            </section>
                        )
                    })}
                </>
            )}
        </section>
    )
}
