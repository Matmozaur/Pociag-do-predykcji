'use client'
import { useQuery } from '@tanstack/react-query'
import { AlertTriangle, ArrowRight, Calendar, CheckCircle2 } from 'lucide-react'
import { gateway, type DisruptionSummaryView } from '@/lib/api'
import { Badge, Button, Card, EmptyState, Spinner } from '@/lib/ui'
import { NavShell } from '@/components/NavShell'

const severityLabel: Record<string, string> = {
    low: 'Niski',
    medium: 'Sredni',
    high: 'Wysoki',
}

const severityVariant: Record<string, 'success' | 'warning' | 'danger'> = {
    low: 'success',
    medium: 'warning',
    high: 'danger',
}

function DisruptionCard({ disruption }: { disruption: DisruptionSummaryView }) {
    return (
        <Card className="flex flex-col gap-3">
            <div className="flex items-start justify-between gap-2">
                <div className="flex items-center gap-2">
                    <AlertTriangle
                        size={16}
                        className={
                            disruption.severity === 'high'
                                ? 'text-red-400'
                                : disruption.severity === 'medium'
                                    ? 'text-amber-400'
                                    : 'text-slate-400'
                        }
                    />
                    {disruption.type_name && <span className="text-xs font-medium text-slate-400">{disruption.type_name}</span>}
                </div>
                {disruption.severity && (
                    <Badge variant={severityVariant[disruption.severity] ?? 'default'}>
                        {severityLabel[disruption.severity] ?? disruption.severity}
                    </Badge>
                )}
            </div>

            {(disruption.start_station || disruption.end_station) && (
                <div className="flex items-center gap-1 text-xs text-slate-400">
                    <span>{disruption.start_station ?? '-'}</span>
                    <ArrowRight size={12} className="text-slate-600" />
                    <span>{disruption.end_station ?? '-'}</span>
                </div>
            )}

            <p className="text-sm leading-relaxed text-slate-300">{disruption.message}</p>

            <div className="flex items-center justify-between text-xs text-slate-500">
                {disruption.date_from && (
                    <div className="flex items-center gap-1">
                        <Calendar size={12} />
                        <span>
                            {disruption.date_from}
                            {disruption.date_to && disruption.date_to !== disruption.date_from ? ` - ${disruption.date_to}` : ''}
                        </span>
                    </div>
                )}
                {disruption.affected_routes_count != null && <span>{disruption.affected_routes_count} tras dotyczy</span>}
            </div>
        </Card>
    )
}

export default function UtrudnieniaPage() {
    const disruptionsQuery = useQuery({
        queryKey: ['disruptions'],
        queryFn: () => gateway.listDisruptions(true),
        staleTime: 120_000,
    })

    return (
        <NavShell title="Utrudnienia">
            <div className="max-w-2xl mx-auto p-4 md:p-6 space-y-4">
                <div>
                    <h1 className="text-xl font-bold text-white">Utrudnienia w ruchu</h1>
                    <p className="text-sm text-slate-500 mt-0.5">Aktywne zakłócenia na sieci kolejowej</p>
                </div>

                {disruptionsQuery.isLoading ? (
                    <div className="flex justify-center py-16">
                        <Spinner className="h-8 w-8" />
                    </div>
                ) : disruptionsQuery.isError ? (
                    <EmptyState
                        title="Wystąpił błąd"
                        description="Nie udało się pobrać informacji o utrudnieniach."
                        action={
                            <Button variant="secondary" onClick={() => disruptionsQuery.refetch()}>
                                Spróbuj ponownie
                            </Button>
                        }
                    />
                ) : disruptionsQuery.data && disruptionsQuery.data.data.length > 0 ? (
                    <div className="space-y-3">
                        {disruptionsQuery.data.data.map((d) => (
                            <DisruptionCard key={d.id} disruption={d} />
                        ))}
                    </div>
                ) : (
                    <EmptyState
                        icon={<CheckCircle2 className="text-green-500" />}
                        title="Brak aktywnych utrudnień"
                        description="Sieć kolejowa działa bez zakłóceń."
                    />
                )}
            </div>
        </NavShell>
    )
}
