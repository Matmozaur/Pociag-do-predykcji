'use client'
import { useParams } from 'next/navigation'
import { useQuery } from '@tanstack/react-query'
import Link from 'next/link'
import { ArrowLeft, CheckCircle2, Circle, XCircle } from 'lucide-react'
import {
    delayVariant,
    formatDelay,
    gateway,
    statusLabel,
    statusVariant,
    type TrainStopView,
} from '@/lib/api'
import { Badge, Button, EmptyState, Spinner, cn } from '@/lib/ui'
import { NavShell } from '@/components/NavShell'

function StopTimeline({ stops }: { stops: TrainStopView[] }) {
    return (
        <ol className="relative">
            {stops.map((stop, idx) => {
                const isLast = idx === stops.length - 1
                const delay = stop.arrival_delay_minutes ?? stop.departure_delay_minutes

                return (
                    <li
                        key={stop.sequence}
                        className={cn('relative flex gap-4 pb-6', isLast && 'pb-0', stop.is_cancelled && 'opacity-40')}
                    >
                        {!isLast && <div className="absolute bottom-0 left-[11px] top-6 w-0.5 bg-[#2d3148]" />}

                        <div className="mt-0.5 flex-shrink-0">
                            {stop.is_cancelled ? (
                                <XCircle size={22} className="text-red-500" />
                            ) : stop.is_confirmed ? (
                                <CheckCircle2 size={22} className="text-green-500" />
                            ) : (
                                <Circle size={22} className="text-slate-600" />
                            )}
                        </div>

                        <div className="min-w-0 flex-1">
                            <div className="flex items-start justify-between gap-2">
                                <p className={cn('text-sm font-medium', stop.is_cancelled ? 'text-slate-500 line-through' : 'text-white')}>
                                    {stop.station_name}
                                </p>
                                {!stop.is_cancelled && (
                                    <Badge variant={delayVariant(delay)}>{formatDelay(delay)}</Badge>
                                )}
                            </div>

                            <div className="mt-1 flex gap-4 text-xs text-slate-500">
                                {stop.planned_arrival && (
                                    <div>
                                        <span className="text-slate-600">Przyj: </span>
                                        <span className={stop.actual_arrival ? 'text-slate-400' : ''}>{stop.planned_arrival}</span>
                                        {stop.actual_arrival && stop.actual_arrival !== stop.planned_arrival && (
                                            <span className="ml-1 text-amber-400">{stop.actual_arrival}</span>
                                        )}
                                    </div>
                                )}
                                {stop.planned_departure && (
                                    <div>
                                        <span className="text-slate-600">Odj: </span>
                                        <span className={stop.actual_departure ? 'text-slate-400' : ''}>{stop.planned_departure}</span>
                                        {stop.actual_departure && stop.actual_departure !== stop.planned_departure && (
                                            <span className="ml-1 text-amber-400">{stop.actual_departure}</span>
                                        )}
                                    </div>
                                )}
                            </div>
                        </div>
                    </li>
                )
            })}
        </ol>
    )
}

export default function TrainDetailPage() {
    const { id } = useParams<{ id: string }>()
    const operationId = id && /^\d+$/.test(id) ? parseInt(id, 10) : null
    const trainQuery = useQuery({
        queryKey: ['trainDetail', operationId],
        queryFn: () => gateway.getTrainDetail(operationId!),
        enabled: operationId !== null,
        refetchInterval: 60_000,
        staleTime: 30_000,
    })

    return (
        <NavShell title={trainQuery.data?.train_name ?? 'Szczegóły pociągu'} showBack>
            <div className="max-w-2xl mx-auto p-4 md:p-6 space-y-4">
                <Link
                    href="/pociagi"
                    className="inline-flex items-center gap-1 text-sm text-slate-500 hover:text-white transition-colors"
                >
                    <ArrowLeft size={14} />
                    Powrót do listy
                </Link>

                {trainQuery.isLoading ? (
                    <div className="flex justify-center py-16">
                        <Spinner className="h-8 w-8" />
                    </div>
                ) : trainQuery.isError ? (
                    <EmptyState
                        title="Wystąpił błąd"
                        description="Nie udało się pobrać danych pociągu."
                        action={
                            <Button variant="secondary" onClick={() => trainQuery.refetch()}>
                                Spróbuj ponownie
                            </Button>
                        }
                    />
                ) : trainQuery.data ? (
                    <>
                        <div className="bg-[#1a1d27] border border-[#2d3148] rounded-xl p-4">
                            <div className="flex items-start justify-between gap-2 flex-wrap">
                                <div>
                                    <h1 className="text-lg font-bold text-white">{trainQuery.data.train_name}</h1>
                                    {trainQuery.data.carrier && (
                                        <p className="text-xs text-slate-500 mt-0.5">{trainQuery.data.carrier.name}</p>
                                    )}
                                    <p className="text-xs text-slate-600 mt-0.5">
                                        Data kursowania: {trainQuery.data.operating_date}
                                    </p>
                                </div>
                                <Badge variant={statusVariant(trainQuery.data.status)}>{statusLabel(trainQuery.data.status)}</Badge>
                            </div>
                        </div>

                        <div>
                            <h2 className="text-sm font-semibold text-slate-400 uppercase tracking-wide mb-4">
                                Przebieg trasy
                            </h2>
                            <StopTimeline stops={trainQuery.data.stops} />
                        </div>
                    </>
                ) : null}
            </div>
        </NavShell>
    )
}
