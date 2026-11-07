'use client'
import { useId, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { addDays, format } from 'date-fns'
import { Gauge } from 'lucide-react'
import {
    api,
    DELAY_PREDICTION_HORIZON_DAYS,
    type DelayRange,
    type ScheduleDetailView,
} from '@/lib/api'
import { Spinner, cn } from '@/lib/ui'

const RANGE_LABELS: Record<DelayRange, string> = {
    '0-5': 'Na czas (do 5 min)',
    '5-10': '5–10 min',
    '10-20': '10–20 min',
    '20-40': '20–40 min',
    '40-60': '40–60 min',
    '60-120': '60–120 min',
    '120+': 'Ponad 120 min',
}

// Green for on time, through amber, to red for the longest delays.
const RANGE_COLORS: Record<DelayRange, string> = {
    '0-5': 'bg-emerald-500',
    '5-10': 'bg-lime-500',
    '10-20': 'bg-yellow-500',
    '20-40': 'bg-amber-500',
    '40-60': 'bg-orange-500',
    '60-120': 'bg-red-500',
    '120+': 'bg-red-700',
}

const selectClass =
    'w-full rounded-lg border border-[#2d3148] bg-[#222536] px-2 py-1.5 text-sm text-slate-100 transition-colors focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500'

function formatPercent(p: number): string {
    const pct = p * 100
    return pct > 0 && pct < 1 ? '<1%' : `${Math.round(pct)}%`
}

/** Predicted arrival delay of one future run of the route, for a chosen journey on it. */
export function DelayPredictionCard({ schedule }: { schedule: ScheduleDetailView }) {
    const uid = useId()
    const lastDate = format(addDays(new Date(), DELAY_PREDICTION_HORIZON_DAYS), 'yyyy-MM-dd')
    const dates = schedule.operating_dates.filter((d) => d <= lastDate)
    const stops = schedule.stops

    const [date, setDate] = useState(dates[0] ?? '')
    const [fromIdx, setFromIdx] = useState(0)
    const [toIdx, setToIdx] = useState(stops.length - 1)

    const params = {
        date,
        from: stops[fromIdx]?.station_external_id,
        to: stops[toIdx]?.station_external_id,
    }
    const predictionQuery = useQuery({
        queryKey: ['delayPrediction', schedule.route_id, params],
        queryFn: () => api.getDelayPrediction(schedule.route_id, params),
        enabled: date !== '' && fromIdx < toIdx,
        staleTime: 5 * 60_000,
    })
    const prediction = predictionQuery.data

    return (
        <section aria-labelledby={`${uid}-title`} className="space-y-4 rounded-xl border border-[#2d3148] bg-[#1a1d27] p-4">
            <div className="flex items-center justify-between gap-2">
                <h2 id={`${uid}-title`} className="flex items-center gap-2 text-sm font-semibold uppercase tracking-wide text-slate-400">
                    <Gauge size={14} aria-hidden="true" /> Prognoza opóźnienia
                </h2>
                {prediction?.is_mock && (
                    <span className="rounded-md bg-amber-500/15 px-2 py-0.5 text-[11px] font-semibold text-amber-300">
                        Dane testowe
                    </span>
                )}
            </div>

            {dates.length === 0 ? (
                <p className="text-sm text-slate-400">
                    Pociąg nie kursuje w najbliższych {DELAY_PREDICTION_HORIZON_DAYS} dniach — prognoza jest niedostępna.
                </p>
            ) : (
                <>
                    <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                        <label className="space-y-1">
                            <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-500">Data</span>
                            <select className={selectClass} value={date} onChange={(e) => setDate(e.target.value)}>
                                {dates.map((d) => (
                                    <option key={d} value={d}>
                                        {d}
                                    </option>
                                ))}
                            </select>
                        </label>
                        <label className="space-y-1">
                            <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-500">Z</span>
                            <select className={selectClass} value={fromIdx} onChange={(e) => setFromIdx(Number(e.target.value))}>
                                {stops.slice(0, -1).map((s, i) => (
                                    <option key={s.order} value={i}>
                                        {s.station_name}
                                    </option>
                                ))}
                            </select>
                        </label>
                        <label className="space-y-1">
                            <span className="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-500">Do</span>
                            <select className={selectClass} value={toIdx} onChange={(e) => setToIdx(Number(e.target.value))}>
                                {stops.map((s, i) =>
                                    i > 0 ? (
                                        <option key={s.order} value={i}>
                                            {s.station_name}
                                        </option>
                                    ) : null,
                                )}
                            </select>
                        </label>
                    </div>

                    {fromIdx >= toIdx ? (
                        <p className="text-sm text-slate-400">Stacja docelowa musi być dalej na trasie niż stacja początkowa.</p>
                    ) : predictionQuery.isLoading ? (
                        <div className="flex justify-center py-6">
                            <Spinner />
                        </div>
                    ) : predictionQuery.isError ? (
                        <div className="rounded-xl border border-red-500/25 bg-red-500/10 p-3 text-sm text-slate-200" role="alert">
                            <p>Nie udało się pobrać prognozy.</p>
                            <button
                                onClick={() => void predictionQuery.refetch()}
                                className="mt-2 text-xs font-semibold text-red-300 underline underline-offset-4 hover:text-white"
                            >
                                Spróbuj ponownie
                            </button>
                        </div>
                    ) : prediction ? (
                        <div className="space-y-3">
                            <p className="text-xs text-slate-500">
                                Opóźnienie przyjazdu do <span className="text-slate-300">{prediction.to.station_name}</span>{' '}
                                (planowo {prediction.to.time}) dla odjazdu z {prediction.from.station_name} o {prediction.from.time}.
                            </p>
                            <ul className="space-y-1.5">
                                {prediction.buckets.map((b) => {
                                    const isBest = b.range === prediction.most_likely
                                    return (
                                        <li key={b.range} className="grid grid-cols-[8.5rem_1fr_3rem] items-center gap-2 text-xs">
                                            <span className={cn('truncate', isBest ? 'font-semibold text-white' : 'text-slate-400')}>
                                                {RANGE_LABELS[b.range]}
                                            </span>
                                            <span className="h-2 overflow-hidden rounded-full bg-[#222536]">
                                                <span
                                                    className={cn('block h-full rounded-full', RANGE_COLORS[b.range])}
                                                    style={{ width: `${b.probability * 100}%` }}
                                                />
                                            </span>
                                            <span className={cn('text-right tabular-nums', isBest ? 'font-semibold text-white' : 'text-slate-400')}>
                                                {formatPercent(b.probability)}
                                            </span>
                                        </li>
                                    )
                                })}
                            </ul>
                            <p className="text-[11px] text-slate-600">
                                Model: {prediction.model_version}
                                {prediction.is_mock && ' · wartości przykładowe, model ML nie jest jeszcze podłączony'}
                            </p>
                        </div>
                    ) : null}
                </>
            )}
        </section>
    )
}
