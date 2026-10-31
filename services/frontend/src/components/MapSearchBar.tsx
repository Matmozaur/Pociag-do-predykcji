'use client'
import { useEffect, useId, useMemo, useRef, useState, type KeyboardEvent } from 'react'
import { useQuery } from '@tanstack/react-query'
import { MapPin, Route, Search, TrainFront, X } from 'lucide-react'
import { api, delayVariant, formatDelay, type StationSuggestion, type TrainMapPoint } from '@/lib/api'
import { matchTrains, parseConnectionQuery, resolveConnectionEnd, type ConnectionEnd } from '@/lib/mapSearch'
import { Badge, Spinner, cn } from '@/lib/ui'

export interface MapSearchBarProps {
    onSelectStation: (station: StationSuggestion) => void
    onSelectTrain: (train: TrainMapPoint) => void
    onSelectConnection: (from: ConnectionEnd, to: ConnectionEnd) => void
    className?: string
}

const DEBOUNCE_MS = 250
const MIN_QUERY_LENGTH = 2
const MAX_STATIONS = 5

type SearchOption =
    | { kind: 'connection'; key: string; label: string; from: ConnectionEnd; to: ConnectionEnd }
    | { kind: 'station'; key: string; label: string; station: StationSuggestion }
    | { kind: 'train'; key: string; label: string; train: TrainMapPoint }

const GROUPS: { kind: SearchOption['kind']; label: string }[] = [
    { kind: 'connection', label: 'Połączenie' },
    { kind: 'station', label: 'Stacje' },
    { kind: 'train', label: 'Pociągi' },
]

export function MapSearchBar({ onSelectStation, onSelectTrain, onSelectConnection, className }: MapSearchBarProps) {
    const [inputValue, setInputValue] = useState('')
    const [debouncedQ, setDebouncedQ] = useState('')
    const [open, setOpen] = useState(false)
    const [activeIndex, setActiveIndex] = useState(-1)
    const containerRef = useRef<HTMLDivElement>(null)
    const inputRef = useRef<HTMLInputElement>(null)
    const baseId = useId()
    const listboxId = `${baseId}-listbox`

    useEffect(() => {
        const timer = setTimeout(() => setDebouncedQ(inputValue.trim()), DEBOUNCE_MS)
        return () => clearTimeout(timer)
    }, [inputValue])

    useEffect(() => {
        function handleOutsideClick(e: PointerEvent) {
            if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
                setOpen(false)
            }
        }

        document.addEventListener('pointerdown', handleOutsideClick)
        return () => document.removeEventListener('pointerdown', handleOutsideClick)
    }, [])

    const hasQuery = debouncedQ.length >= MIN_QUERY_LENGTH
    const connection = useMemo(() => parseConnectionQuery(debouncedQ), [debouncedQ])
    const isConnection = hasQuery && connection != null

    const stationsQuery = useQuery({
        queryKey: ['stations', debouncedQ],
        queryFn: () => api.searchStations(debouncedQ),
        enabled: hasQuery && !isConnection,
        staleTime: 60_000,
    })

    const fromQ = connection?.from ?? ''
    const toQ = connection?.to ?? ''
    const fromQuery = useQuery({
        queryKey: ['stations', fromQ],
        queryFn: () => api.searchStations(fromQ),
        enabled: isConnection,
        staleTime: 60_000,
    })
    const toQuery = useQuery({
        queryKey: ['stations', toQ],
        queryFn: () => api.searchStations(toQ),
        enabled: isConnection,
        staleTime: 60_000,
    })

    // The map keeps this key populated; a disabled query still serves what is cached.
    const trainsQuery = useQuery({
        queryKey: ['mapTrains'],
        queryFn: () => api.getMapTrains(),
        staleTime: 15_000,
        enabled: hasQuery && !isConnection,
    })

    const options = useMemo<SearchOption[]>(() => {
        if (!hasQuery) return []
        if (isConnection) {
            const from = resolveConnectionEnd(fromQuery.data?.suggestions ?? [], fromQ)
            const to = resolveConnectionEnd(toQuery.data?.suggestions ?? [], toQ)
            if (!from || !to || from.query.toLowerCase() === to.query.toLowerCase()) return []
            return [{ kind: 'connection', key: `connection-${from.query}-${to.query}`, label: `${from.label} → ${to.label}`, from, to }]
        }
        const stations: SearchOption[] = (stationsQuery.data?.suggestions ?? [])
            .slice(0, MAX_STATIONS)
            .map((station) => ({ kind: 'station', key: `station-${station.external_id}`, label: station.name, station }))
        const trains: SearchOption[] = matchTrains(trainsQuery.data?.trains ?? [], debouncedQ)
            .map((train) => ({ kind: 'train', key: `train-${train.operation_id}`, label: train.train_name, train }))
        return [...stations, ...trains]
    }, [hasQuery, isConnection, fromQuery.data, toQuery.data, fromQ, toQ, stationsQuery.data, trainsQuery.data, debouncedQ])

    const isFetching = isConnection
        ? fromQuery.isFetching || toQuery.isFetching
        : hasQuery && (stationsQuery.isFetching || trainsQuery.isFetching)
    const isPending = inputValue.trim() !== debouncedQ || (isConnection
        ? fromQuery.isLoading || toQuery.isLoading
        : stationsQuery.isLoading || (trainsQuery.isLoading && !trainsQuery.data))
    const stationError = isConnection ? fromQuery.isError || toQuery.isError : stationsQuery.isError
    const showDropdown = open && hasQuery && inputValue.trim().length >= MIN_QUERY_LENGTH
    const currentActive = activeIndex < options.length ? activeIndex : -1
    const activeOptionId = showDropdown && currentActive >= 0 ? `${baseId}-option-${currentActive}` : undefined

    function select(option: SearchOption) {
        setOpen(false)
        setActiveIndex(-1)
        setInputValue(option.label)
        // A train pick has no panel heading to take focus, so keep it in the input.
        if (option.kind !== 'train') inputRef.current?.blur()
        if (option.kind === 'station') onSelectStation(option.station)
        else if (option.kind === 'train') onSelectTrain(option.train)
        else onSelectConnection(option.from, option.to)
    }

    function clear() {
        setInputValue('')
        setDebouncedQ('')
        setActiveIndex(-1)
        setOpen(false)
        inputRef.current?.focus()
    }

    function handleKeyDown(e: KeyboardEvent<HTMLInputElement>) {
        if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
            e.preventDefault()
            if (!showDropdown) {
                setOpen(true)
                return
            }
            if (!options.length) return
            const step = e.key === 'ArrowDown' ? 1 : -1
            const start = currentActive < 0 ? (step === 1 ? -1 : 0) : currentActive
            setActiveIndex((start + step + options.length) % options.length)
        } else if (e.key === 'Enter') {
            // Options still belong to the previous query while the debounce runs: flush it instead.
            const trimmed = inputValue.trim()
            if (trimmed !== debouncedQ) {
                e.preventDefault()
                setDebouncedQ(trimmed)
                setOpen(true)
                return
            }
            if (!showDropdown || !options.length) return
            e.preventDefault()
            select(options[currentActive >= 0 ? currentActive : 0])
        } else if (e.key === 'Escape') {
            // React's stopPropagation also stops the native event before it reaches window
            // listeners (React 17+ listens at the root), so the map page's Escape handler stays quiet.
            if (showDropdown) {
                e.stopPropagation()
                setOpen(false)
                setActiveIndex(-1)
            } else if (inputValue) {
                e.stopPropagation()
                clear()
            }
        }
    }

    let optionIndex = -1

    return (
        <div ref={containerRef} className={cn('pointer-events-auto relative w-full', className)}>
            <div className="relative">
                <Search size={16} className="pointer-events-none absolute left-3.5 top-1/2 z-10 -translate-y-1/2 text-slate-500" aria-hidden="true" />
                <input
                    ref={inputRef}
                    type="text"
                    role="combobox"
                    aria-label="Szukaj stacji, pociągu lub połączenia"
                    aria-expanded={showDropdown}
                    aria-controls={listboxId}
                    aria-autocomplete="list"
                    aria-activedescendant={activeOptionId}
                    autoComplete="off"
                    spellCheck={false}
                    placeholder="Stacja, pociąg lub A - B"
                    value={inputValue}
                    onChange={(e) => {
                        setInputValue(e.target.value)
                        setActiveIndex(-1)
                        setOpen(true)
                    }}
                    onFocus={() => setOpen(true)}
                    onBlur={() => setOpen(false)}
                    onKeyDown={handleKeyDown}
                    className="h-11 w-full rounded-xl border border-white/10 bg-[#121622]/95 pl-10 pr-16 text-base text-slate-100 shadow-lg shadow-black/30 backdrop-blur placeholder:text-slate-500 transition-colors focus-visible:border-blue-400/60 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400 md:text-sm"
                />
                <div className="absolute right-2 top-1/2 z-10 flex -translate-y-1/2 items-center gap-1">
                    {isFetching && <Spinner className="h-4 w-4" />}
                    {inputValue && (
                        <button
                            type="button"
                            onClick={clear}
                            aria-label="Wyczyść wyszukiwanie"
                            className="flex h-7 w-7 items-center justify-center rounded-md text-slate-400 transition-colors hover:bg-white/5 hover:text-white focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400"
                        >
                            <X size={15} aria-hidden="true" />
                        </button>
                    )}
                </div>
            </div>

            {/* Keeps the input focused (and the dropdown open) when the panel's scrollbar is dragged. */}
            <div
                hidden={!showDropdown}
                onMouseDown={(e) => e.preventDefault()}
                className="absolute top-full z-[600] mt-1.5 max-h-[60vh] w-full overflow-y-auto overscroll-contain rounded-xl border border-white/10 bg-[#121622]/95 p-1.5 shadow-lg shadow-black/30 backdrop-blur"
            >
                <div role="status" aria-live="polite">
                    {showDropdown && stationError && (
                        <p className="px-2.5 py-2 text-xs text-red-300">Nie udało się wyszukać stacji.</p>
                    )}
                    {showDropdown && !options.length && !isPending && !isFetching && !stationError && (
                        <p className="px-2.5 py-2 text-sm text-slate-400">Brak wyników dla „{debouncedQ}”.</p>
                    )}
                    {showDropdown && !options.length && (isPending || isFetching) && (
                        <p className="px-2.5 py-2 text-sm text-slate-500">Szukam…</p>
                    )}
                </div>
                <div id={listboxId} role="listbox" aria-label="Wyniki wyszukiwania">
                    {showDropdown && GROUPS.map((group) => {
                        const groupOptions = options.filter((option) => option.kind === group.kind)
                        if (!groupOptions.length) return null
                        return (
                            <div key={group.kind} role="group" aria-labelledby={`${baseId}-group-${group.kind}`} className="py-1">
                                <div id={`${baseId}-group-${group.kind}`} role="presentation" className="px-2.5 pb-1 pt-1.5 text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-500">
                                    {group.label}
                                </div>
                                {groupOptions.map((option) => {
                                    optionIndex += 1
                                    const index = optionIndex
                                    const active = index === currentActive
                                    return (
                                        <div
                                            key={option.key}
                                            id={`${baseId}-option-${index}`}
                                            role="option"
                                            aria-selected={active}
                                            onMouseDown={(e) => e.preventDefault()}
                                            onMouseEnter={() => setActiveIndex(index)}
                                            onClick={() => select(option)}
                                            className={cn(
                                                'flex cursor-pointer items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm transition-colors',
                                                active ? 'bg-blue-500/15 text-white' : 'text-slate-200 hover:bg-white/5',
                                            )}
                                        >
                                            <OptionContent option={option} />
                                        </div>
                                    )
                                })}
                            </div>
                        )
                    })}
                </div>
            </div>
        </div>
    )
}

function OptionContent({ option }: { option: SearchOption }) {
    if (option.kind === 'connection') {
        return (
            <>
                <Route size={15} className="flex-none text-blue-300" aria-hidden="true" />
                <span className="min-w-0 flex-1 truncate">
                    <span className="text-slate-400">Połączenie: </span>
                    <span className="font-medium">{option.label}</span>
                </span>
            </>
        )
    }

    if (option.kind === 'station') {
        const { station } = option
        return (
            <>
                <MapPin size={15} className="flex-none text-slate-500" aria-hidden="true" />
                <span className="min-w-0 flex-1 truncate">
                    <span>{station.name}</span>
                    {station.city && station.city !== station.name && (
                        <span className="ml-1.5 text-xs text-slate-500">{station.city}</span>
                    )}
                </span>
            </>
        )
    }

    const { train } = option
    return (
        <>
            <TrainFront size={15} className="flex-none text-blue-300" aria-hidden="true" />
            <span className="min-w-0 flex-1">
                <span className="block truncate font-medium">{train.train_name}</span>
                {(train.origin || train.destination) && (
                    <span className="block truncate text-xs text-slate-500">
                        {train.origin ?? '?'} → {train.destination ?? '?'}
                    </span>
                )}
            </span>
            {train.delay_minutes != null && (
                <Badge variant={delayVariant(train.delay_minutes)} className="flex-none tabular-nums">
                    {formatDelay(train.delay_minutes)}
                </Badge>
            )}
        </>
    )
}
