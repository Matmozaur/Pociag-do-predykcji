'use client'
import { useInfiniteQuery, useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import Link from 'next/link'
import { ArrowRight, MapPin, RefreshCw, Train } from 'lucide-react'
import { delayVariant, formatDelay, api, statusLabel, statusVariant, type LiveTrainSummary } from '@/lib/api'
import { Badge, Button, Card, EmptyState, Spinner } from '@/lib/ui'
import { NavShell } from '@/components/NavShell'
import { DataFreshness } from '@/components/DataFreshness'

const PAGE_SIZE = 100

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
    const trainsQuery = useInfiniteQuery({
        queryKey: ['liveTrainsByDelay'],
        queryFn: ({ pageParam }) => api.getLiveTrains({ sort: 'delay', limit: PAGE_SIZE, offset: pageParam }),
        initialPageParam: 0,
        getNextPageParam: ({ pagination }) => (pagination.has_more ? pagination.offset + pagination.limit : undefined),
        refetchInterval: 60_000,
        staleTime: 30_000,
    })
    // Pages are separate queries, so a train can shift across a page boundary; keep its first copy,
    // whose delay matches the position it is listed at.
    const sortedTrains = useMemo(() => {
        const seen = new Set<number>()
        return (trainsQuery.data?.pages.flatMap((page) => page.data) ?? []).filter(
            (train) => !seen.has(train.operation_id) && seen.add(train.operation_id),
        )
    }, [trainsQuery.data])
    const total = trainsQuery.data?.pages.at(-1)?.pagination.total
    const overviewQuery = useQuery({
        queryKey: ['dashboardOverview'],
        queryFn: api.getDashboardOverview,
        refetchInterval: 60_000,
        staleTime: 30_000,
    })

    return (
        <NavShell title="Pociągi na żywo">
            <div className="max-w-2xl mx-auto p-4 md:p-6 space-y-4">
                <div className="flex items-center justify-between">
                    <div>
                        <h1 className="text-xl font-bold text-white">Pociągi na żywo</h1>
                        <DataFreshness
                            lastUpdated={overviewQuery.data?.data_freshness.operations_last_updated}
                            className="mt-0.5"
                        />
                    </div>
                    <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                            void trainsQuery.refetch()
                            void overviewQuery.refetch()
                        }}
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
                ) : sortedTrains.length > 0 ? (
                    <div className="space-y-3">
                        <p className="text-xs text-slate-500">
                            {total} pociągów · auto-odświeżanie co 60s
                        </p>
                        {sortedTrains.map((train) => (
                            <LiveTrainCard key={train.operation_id} train={train} />
                        ))}
                        {trainsQuery.hasNextPage && (
                            <Button
                                variant="secondary"
                                className="w-full"
                                onClick={() => void trainsQuery.fetchNextPage()}
                                disabled={trainsQuery.isFetchingNextPage}
                            >
                                Pokaż więcej
                            </Button>
                        )}
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
