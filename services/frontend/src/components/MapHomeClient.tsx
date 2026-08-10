'use client'

import { useQuery } from '@tanstack/react-query'
import Link from 'next/link'
import {
    AlertTriangle,
    ArrowRight,
    CheckCircle2,
    RefreshCw,
    Signal,
    Train,
} from 'lucide-react'
import { TrafficMapClient } from '@/components/TrafficMapClient'
import {
    delayVariant,
    formatDelay,
    gateway,
    statusLabel,
    type DisruptionSummaryView,
    type LiveTrainSummary,
} from '@/lib/api'
import { Badge, Button, Spinner } from '@/lib/ui'

function retryLabel(isFetching: boolean) {
    return isFetching ? 'Odświeżanie danych' : 'Odśwież dane'
}

function QueryMessage({ children, onRetry }: { children: React.ReactNode; onRetry: () => void }) {
    return (
        <div className="rounded-xl border border-red-500/25 bg-red-500/10 p-3 text-sm text-slate-200" role="alert">
            <p>{children}</p>
            <button onClick={onRetry} className="mt-2 text-xs font-semibold text-red-300 underline underline-offset-4 hover:text-white">
                Spróbuj ponownie
            </button>
        </div>
    )
}

function Metric({ label, value, tone = 'text-white' }: { label: string; value: string | number; tone?: string }) {
    return (
        <div className="border-l border-white/10 pl-3 first:border-l-0 first:pl-0">
            <p className="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-500">{label}</p>
            <p className={`mt-1 text-lg font-semibold tabular-nums ${tone}`}>{value}</p>
        </div>
    )
}

function TrainRow({ train }: { train: LiveTrainSummary }) {
    return (
        <Link href={`/pociagi/${train.operation_id}`} className="group flex items-center gap-3 rounded-lg px-2 py-2.5 transition-colors hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400">
            <span className="flex h-8 w-8 flex-none items-center justify-center rounded-full bg-blue-500/15 text-blue-300"><Train size={15} /></span>
            <span className="min-w-0 flex-1">
                <span className="block truncate text-sm font-medium text-slate-100">{train.train_name}</span>
                <span className="block truncate text-xs text-slate-500">{train.current_station ? `Przy: ${train.current_station}` : train.next_station ? `Następnie: ${train.next_station}` : 'Pozycja w trakcie aktualizacji'}</span>
            </span>
            <span className="flex flex-col items-end gap-1">
                <Badge variant={delayVariant(train.delay_minutes)}>{formatDelay(train.delay_minutes)}</Badge>
                <span className="text-[10px] text-slate-500">{statusLabel(train.status)}</span>
            </span>
        </Link>
    )
}

function DisruptionRow({ disruption }: { disruption: DisruptionSummaryView }) {
    const critical = disruption.severity === 'high'
    return (
        <Link href="/utrudnienia" className="group flex gap-3 rounded-lg px-2 py-2.5 transition-colors hover:bg-white/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400">
            <AlertTriangle size={16} className={`mt-0.5 flex-none ${critical ? 'text-red-400' : 'text-amber-400'}`} aria-hidden="true" />
            <span className="min-w-0 flex-1">
                <span className="block truncate text-sm text-slate-200">{disruption.type_name ?? 'Utrudnienie w ruchu'}</span>
                <span className="block truncate text-xs text-slate-500">{disruption.start_station && disruption.end_station ? `${disruption.start_station} — ${disruption.end_station}` : disruption.message}</span>
            </span>
            <ArrowRight size={15} className="mt-1 text-slate-600 transition-transform group-hover:translate-x-0.5" aria-hidden="true" />
        </Link>
    )
}

export function MapHomeClient() {
    const overviewQuery = useQuery({ queryKey: ['dashboardOverview'], queryFn: gateway.getDashboardOverview, staleTime: 30_000 })
    const trainsQuery = useQuery({ queryKey: ['mapLiveTrains'], queryFn: () => gateway.getLiveTrains({ limit: 4 }), staleTime: 30_000, refetchInterval: 60_000 })
    const disruptionsQuery = useQuery({ queryKey: ['mapDisruptions'], queryFn: () => gateway.listDisruptions(true, 3), staleTime: 60_000 })
    const isRefreshing = overviewQuery.isFetching || trainsQuery.isFetching || disruptionsQuery.isFetching
    const refreshAll = () => { void Promise.all([overviewQuery.refetch(), trainsQuery.refetch(), disruptionsQuery.refetch()]) }

    return (
        <div className="map-home h-full w-full bg-[#0a0c14]">
            <TrafficMapClient />
            <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(90deg,rgba(10,12,20,.48),transparent_45%),linear-gradient(0deg,rgba(10,12,20,.35),transparent_40%)]" aria-hidden="true" />

            <section aria-labelledby="map-home-title" className="map-operations-panel absolute inset-x-0 bottom-[calc(3.5rem+env(safe-area-inset-bottom))] z-[500] mx-2 max-h-[min(52%,28rem)] touch-pan-y overflow-y-auto overscroll-contain rounded-t-2xl border border-white/10 bg-[#121622]/95 p-4 shadow-2xl shadow-black/40 backdrop-blur-xl md:inset-x-auto md:bottom-auto md:left-5 md:top-5 md:mx-0 md:max-h-[calc(100%-2.5rem)] md:w-[390px] md:rounded-2xl md:p-5">
                <div className="mx-auto mb-3 h-1 w-10 rounded-full bg-white/20 md:hidden" aria-hidden="true" />
                <div className="mb-5 flex items-start justify-between gap-3">
                    <div>
                        <div className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-blue-300"><Signal size={14} /> Centrum operacyjne</div>
                        <h1 id="map-home-title" className="text-xl font-semibold tracking-tight text-white">Stan sieci kolejowej</h1>
                        <p className="mt-1 text-sm text-slate-400">Przegląd ruchu i aktywnych utrudnień.</p>
                    </div>
                    <Button variant="ghost" size="sm" onClick={refreshAll} disabled={isRefreshing} aria-label={retryLabel(isRefreshing)} title={retryLabel(isRefreshing)} className="mt-0.5 h-9 w-9 !px-0">
                        <RefreshCw size={16} className={isRefreshing ? 'animate-spin' : ''} />
                    </Button>
                </div>

                <section aria-label="Najważniejsze wskaźniki" className="mb-5">
                    {overviewQuery.isLoading ? <div className="grid grid-cols-3 gap-3" aria-label="Ładowanie wskaźników"><div className="h-12 animate-pulse rounded bg-white/5" /><div className="h-12 animate-pulse rounded bg-white/5" /><div className="h-12 animate-pulse rounded bg-white/5" /></div> : overviewQuery.isError ? <QueryMessage onRetry={() => overviewQuery.refetch()}>Nie udało się pobrać podsumowania sieci.</QueryMessage> : overviewQuery.data ? (
                        <div className="grid grid-cols-3 gap-2 rounded-xl border border-white/8 bg-black/15 p-3">
                            <Metric label="W ruchu" value={overviewQuery.data.statistics.in_progress ?? '—'} tone="text-emerald-300" />
                            <Metric label="Śr. opóźnienie" value={overviewQuery.data.statistics.avg_delay_minutes == null ? '—' : `${overviewQuery.data.statistics.avg_delay_minutes.toFixed(2)} min`} tone="text-amber-300" />
                            <Metric label="Utrudnienia" value={overviewQuery.data.disruptions_active} tone={overviewQuery.data.disruptions_active > 0 ? 'text-red-300' : 'text-emerald-300'} />
                        </div>
                    ) : null}
                </section>

                <section aria-labelledby="live-trains-heading" className="border-t border-white/8 pt-4">
                    <div className="mb-1 flex items-center justify-between"><h2 id="live-trains-heading" className="text-sm font-semibold text-white">Pociągi w ruchu</h2><Link href="/pociagi" className="text-xs font-medium text-blue-300 hover:text-blue-200">Zobacz wszystkie</Link></div>
                    {trainsQuery.isLoading ? <div className="flex justify-center py-5"><Spinner /></div> : trainsQuery.isError ? <QueryMessage onRetry={() => trainsQuery.refetch()}>Nie udało się pobrać pociągów.</QueryMessage> : trainsQuery.data?.data.length ? <div>{trainsQuery.data.data.map((train) => <TrainRow key={train.operation_id} train={train} />)}</div> : <div className="flex items-center gap-2 py-4 text-sm text-slate-400"><CheckCircle2 size={17} className="text-emerald-400" /> Brak pociągów w ruchu.</div>}
                </section>

                <section aria-labelledby="disruptions-heading" className="mt-3 border-t border-white/8 pt-4">
                    <div className="mb-1 flex items-center justify-between"><h2 id="disruptions-heading" className="text-sm font-semibold text-white">Aktywne utrudnienia</h2><Link href="/utrudnienia" className="text-xs font-medium text-blue-300 hover:text-blue-200">Pełna lista</Link></div>
                    {disruptionsQuery.isLoading ? <div className="flex justify-center py-5"><Spinner /></div> : disruptionsQuery.isError ? <QueryMessage onRetry={() => disruptionsQuery.refetch()}>Nie udało się pobrać utrudnień.</QueryMessage> : disruptionsQuery.data?.data.length ? <div>{disruptionsQuery.data.data.map((disruption) => <DisruptionRow key={disruption.id} disruption={disruption} />)}</div> : <div className="flex items-center gap-2 py-4 text-sm text-slate-400"><CheckCircle2 size={17} className="text-emerald-400" /> Brak aktywnych utrudnień.</div>}
                </section>
                <p className="mt-4 border-t border-white/8 pt-3 text-[11px] leading-relaxed text-slate-500">Mapa pokazuje infrastrukturę kolejową. Dane operacyjne są prezentowane na liście, bez przybliżania pozycji pociągów.</p>
            </section>
            <div className="pointer-events-none absolute right-4 top-4 z-[400] hidden rounded-full border border-white/10 bg-[#121622]/85 px-3 py-1.5 text-xs text-slate-300 shadow-lg backdrop-blur md:block">Warstwa infrastruktury kolejowej</div>
        </div>
    )
}
