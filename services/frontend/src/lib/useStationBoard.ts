'use client'

import { useQuery } from '@tanstack/react-query'
import { ApiError, api } from '@/lib/api'

/** Live station board (Na stacji / Przyjazdy / Odjazdy), refreshed every minute while visible. */
export function useStationBoard(id: number | null | undefined, limit: number) {
    return useQuery({
        queryKey: ['stationBoard', id, limit],
        queryFn: () => api.getStationBoard(id!, limit),
        enabled: id != null,
        staleTime: 30_000,
        refetchInterval: 60_000,
        refetchIntervalInBackground: false,
        // Keep the current rows on screen while "Pokaż więcej" loads a bigger page.
        placeholderData: (prev) => (prev?.station.external_id === id ? prev : undefined),
        // A 404 (unknown station) will not fix itself; don't retry it.
        retry: (failureCount, error) => !(error instanceof ApiError && error.status === 404) && failureCount < 1,
    })
}
