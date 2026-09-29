'use client'

import { formatDistanceToNowStrict } from 'date-fns'
import { pl } from 'date-fns/locale'
import { Badge } from '@/lib/ui'

const STALE_AFTER_MS = 20 * 60_000

const warsawClock = new Intl.DateTimeFormat('pl-PL', {
    timeZone: 'Europe/Warsaw',
    hour: '2-digit',
    minute: '2-digit',
})

/** Shows when operations data was last refreshed from PLK, with a warning when it is stale. */
export function DataFreshness({ lastUpdated, className }: { lastUpdated?: string; className?: string }) {
    if (!lastUpdated) return null
    const updatedAt = new Date(lastUpdated)
    if (Number.isNaN(updatedAt.getTime())) return null
    const isStale = Date.now() - updatedAt.getTime() > STALE_AFTER_MS

    return (
        <div className={className}>
            <p className="text-xs text-slate-500">Dane z {warsawClock.format(updatedAt)}</p>
            {isStale && (
                <Badge variant="warning" className="mt-1 rounded-lg">
                    Dane mogą być nieaktualne (ostatnia aktualizacja {formatDistanceToNowStrict(updatedAt, { locale: pl, addSuffix: true })})
                </Badge>
            )}
        </div>
    )
}
