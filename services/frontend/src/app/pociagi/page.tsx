'use client'
import { useQuery } from '@tanstack/react-query'
import { format } from 'date-fns'
import { pl } from 'date-fns/locale'
import Link from 'next/link'
import { ArrowRight, MapPin, RefreshCw, Train } from 'lucide-react'
import { delayVariant, formatDelay, gateway, statusLabel, statusVariant, type LiveTrainSummary } from '@/lib/api'
import { Badge, Button, Card, EmptyState, Spinner } from '@/lib/ui'
import { NavShell } from '@/components/NavShell'

function LiveTrainCard({ train }: { train: LiveTrainSummary }) {
    return (
        <Link href={`/pociagi/${train.operation_id}`} className="block">
            <Card className="flex flex-col gap-2">
                <div className="flex items-start justify-between gap-2">
                    <div>
                        <p className="text-sm font-semibold text-white">{train.train_name}</p>
                        {train.carrier_code && <span className="text-xs text-slate-500">{train.carrier_code}</span>}
                    </div>
                    <div className="flex flex-shrink-0 items-center gap-2">
                        <Badge variant={statusVariant(train.status)}>{statusLabel(train.status)}</Badge>
                        <Badge variant={delayVariant(train.delay_minutes)}>{formatDelay(train.delay_minutes)}</Badge>
                    </div>
                </div>

                {(train.origin || train.destination) && (
                    <div className="flex items-center gap-1 text-xs text-slate-400">
                        <span>{train.origin ?? '-'}</span>
                        <ArrowRight size={12} className="text-slate-600" />
                        <span>{train.destination ?? '-'}</span>
                    </div>
                )}

                {(train.current_station || train.next_station) && (
                    <div className="flex items-center gap-1 text-xs text-slate-500">
                        <MapPin size={12} />
                        {train.current_station && <span>Aktualnie: {train.current_station}</span>}
                        {train.next_station && <span className="ml-1">→ {train.next_station}</span>}
                    </div>
                )}
            </Card>
        </Link>
    )
}

export default function PociagiPage() {
    const trainsQuery = useQuery({
        queryKey: ['liveTrains', ''],
        queryFn: () => gateway.getLiveTrains({ carriers: undefined, limit: 50 }),
        refetchInterval: 60_000,
        staleTime: 30_000,
    })

    return (
        <NavShell title="Pociągi na żywo">
            <div className="max-w-2xl mx-auto p-4 md:p-6 space-y-4">
                <div className="flex items-center justify-between">
                    <div>
                        <h1 className="text-xl font-bold text-white">Pociągi na żywo</h1>
                        {trainsQuery.dataUpdatedAt > 0 && (
                            <p className="text-xs text-slate-500 mt-0.5">
                                Odświeżono: {format(trainsQuery.dataUpdatedAt, 'HH:mm:ss', { locale: pl })}
                            </p>
                        )}
                    </div>
                    <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => trainsQuery.refetch()}
                        disabled={trainsQuery.isFetching}
                    >
                        <RefreshCw size={14} className={trainsQuery.isFetching ? 'animate-spin' : ''} />
                    </Button>
                </div>

                {trainsQuery.isLoading ? (
                    <div className="flex justify-center py-16">
                        <Spinner className="h-8 w-8" />
                    </div>
                ) : trainsQuery.isError ? (
                    <EmptyState
                        title="Wystąpił błąd"
                        description="Nie udało się pobrać danych o pociągach."
                        action={
                            <Button variant="secondary" onClick={() => trainsQuery.refetch()}>
                                Spróbuj ponownie
                            </Button>
                        }
                    />
                ) : trainsQuery.data && trainsQuery.data.data.length > 0 ? (
                    <div className="space-y-3">
                        <p className="text-xs text-slate-500">
                            {trainsQuery.data.data.length} pociągów · auto-odświeżanie co 60s
                        </p>
                        {trainsQuery.data.data.map((train) => (
                            <LiveTrainCard key={train.operation_id} train={train} />
                        ))}
                    </div>
                ) : (
                    <EmptyState
                        icon={<Train />}
                        title="Brak aktywnych pociągów"
                        description="Nie ma aktualnie pociągów w ruchu."
                    />
                )}
            </div>
        </NavShell>
    )
}
