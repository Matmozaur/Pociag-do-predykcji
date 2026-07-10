'use client'
import { useParams } from 'next/navigation'
import { useQuery } from '@tanstack/react-query'
import { Clock, Calendar, ArrowLeft } from 'lucide-react'
import Link from 'next/link'
import { gateway } from '@/lib/api'
import { Button, EmptyState, Spinner } from '@/lib/ui'
import { NavShell } from '@/components/NavShell'

const CARRIER_COLORS: Record<string, string> = {
    IC: 'bg-red-500/20 text-red-400',
    TLK: 'bg-orange-500/20 text-orange-400',
    KM: 'bg-purple-500/20 text-purple-400',
    REG: 'bg-slate-500/20 text-slate-400',
}

export default function RozkladDetailPage() {
    const { id } = useParams<{ id: string }>()
    const routeId = id && /^\d+$/.test(id) ? parseInt(id, 10) : null
    const scheduleQuery = useQuery({
        queryKey: ['scheduleDetail', routeId],
        queryFn: () => gateway.getScheduleDetail(routeId!),
        enabled: routeId !== null,
        staleTime: 300_000,
    })

    const category = scheduleQuery.data?.commercial_category ?? scheduleQuery.data?.carrier.code ?? ''
    const colorClass = CARRIER_COLORS[category] ?? 'bg-blue-500/20 text-blue-400'

    return (
        <NavShell title={scheduleQuery.data?.train_name ?? 'Rozkład jazdy'} showBack>
            <div className="max-w-2xl mx-auto p-4 md:p-6 space-y-4">
                <Link
                    href="/wyszukaj"
                    className="inline-flex items-center gap-1 text-sm text-slate-500 hover:text-white transition-colors"
                >
                    <ArrowLeft size={14} />
                    Powrót do wyszukiwania
                </Link>

                {scheduleQuery.isLoading ? (
                    <div className="flex justify-center py-16">
                        <Spinner className="h-8 w-8" />
                    </div>
                ) : scheduleQuery.isError ? (
                    <EmptyState
                        title="Wystąpił błąd"
                        description="Nie udało się załadować rozkładu jazdy."
                        action={
                            <Button variant="secondary" onClick={() => scheduleQuery.refetch()}>
                                Spróbuj ponownie
                            </Button>
                        }
                    />
                ) : scheduleQuery.data ? (
                    <>
                        {/* Route header */}
                        <div className="bg-[#1a1d27] border border-[#2d3148] rounded-xl p-4 space-y-3">
                            <div className="flex items-start justify-between gap-2">
                                <div>
                                    <h1 className="text-lg font-bold text-white">{scheduleQuery.data.train_name}</h1>
                                    <p className="text-sm text-slate-500 mt-0.5">{scheduleQuery.data.carrier.name}</p>
                                </div>
                                <span className={`text-xs font-bold px-2 py-1 rounded-md ${colorClass}`}>
                                    {category}
                                </span>
                            </div>

                            {scheduleQuery.data.total_duration_minutes != null && (
                                <div className="flex items-center gap-1 text-xs text-slate-500">
                                    <Clock size={12} />
                                    <span>
                                        Czas przejazdu:{' '}
                                        {Math.floor(scheduleQuery.data.total_duration_minutes / 60)}h{' '}
                                        {scheduleQuery.data.total_duration_minutes % 60 > 0
                                            ? `${scheduleQuery.data.total_duration_minutes % 60} min`
                                            : ''}
                                    </span>
                                </div>
                            )}

                            {scheduleQuery.data.national_number && (
                                <p className="text-xs text-slate-500">Nr pociągu: {scheduleQuery.data.national_number}</p>
                            )}

                            {scheduleQuery.data.operating_dates.length > 0 && (
                                <div className="flex items-start gap-1 text-xs text-slate-500">
                                    <Calendar size={12} className="mt-0.5 flex-shrink-0" />
                                    <div>
                                        <span>Kursuje: {scheduleQuery.data.operating_dates.slice(0, 5).join(', ')}</span>
                                        {scheduleQuery.data.operating_dates.length > 5 && (
                                            <span> i {scheduleQuery.data.operating_dates.length - 5} więcej dni...</span>
                                        )}
                                    </div>
                                </div>
                            )}
                        </div>

                        {/* Stops list */}
                        <div>
                            <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-wide mb-3">
                                Stacje · {scheduleQuery.data.stops.length} przystanków
                            </h2>
                            <div className="relative">
                                {scheduleQuery.data.stops.map((stop, idx) => {
                                    const isFirst = idx === 0
                                    const isLast = idx === scheduleQuery.data.stops.length - 1

                                    return (
                                        <div key={stop.order} className="flex gap-3 pb-4 relative">
                                            {!isLast && (
                                                <div className="absolute left-[9px] top-5 bottom-0 w-0.5 bg-[#2d3148]" />
                                            )}
                                            <div className="flex-shrink-0 mt-0.5">
                                                {isFirst || isLast ? (
                                                    <div className="h-[18px] w-[18px] rounded-full border-2 border-blue-500 bg-[#0f1117]" />
                                                ) : (
                                                    <div className="h-[18px] w-[18px] rounded-full border-2 border-[#2d3148] bg-[#0f1117]" />
                                                )}
                                            </div>
                                            <div className="flex-1 min-w-0 pb-1">
                                                <p
                                                    className={`text-sm font-medium ${isFirst || isLast ? 'text-white' : 'text-slate-300'
                                                        }`}
                                                >
                                                    {stop.station_name}
                                                </p>
                                                <div className="flex gap-3 mt-0.5 text-xs text-slate-500">
                                                    {stop.arrival_time && <span>Przyjazd: {stop.arrival_time}</span>}
                                                    {stop.departure_time && <span>Odjazd: {stop.departure_time}</span>}
                                                    {stop.platform && <span>Peron: {stop.platform}</span>}
                                                </div>
                                            </div>
                                        </div>
                                    )
                                })}
                            </div>
                        </div>
                    </>
                ) : null}
            </div>
        </NavShell>
    )
}
