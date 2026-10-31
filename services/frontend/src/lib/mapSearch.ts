// Matching helpers for the /mapa search bar: text normalisation, "A - B" connection parsing and
// ranking of live map trains. Pure functions, no React — the component is MapSearchBar.tsx.

import type { StationSuggestion, TrainMapPoint } from '@/lib/api'

/** Lowercase, strip Polish diacritics, collapse whitespace and trim. */
export function normalizeText(s: string): string {
    return s
        .toLowerCase()
        .replace(/ł/g, 'l')
        .normalize('NFD')
        .replace(/[\u0300-\u036f]/g, '')
        .replace(/\s+/g, ' ')
        .trim()
}

// "A -> B", "A → B", "A > B", "A – B", "A — B". The plain hyphen is handled separately so
// that hyphenated names such as "Bielsko-Biała" are never split.
const ARROW_SEPARATOR = /^(.+?)\s*(?:->|→|>|–|—)\s*(.+)$/
const HYPHEN_SEPARATOR = /^(.+?)\s+-\s+(.+)$/
const DO_SEPARATOR = /^(?:z\s+)?(.+?)\s+do\s+(.+)$/i

/** Recognises a "from → to" connection query; null when the query is a plain search. */
export function parseConnectionQuery(q: string): { from: string; to: string } | null {
    const query = q.replace(/\s+/g, ' ').trim()
    const match = ARROW_SEPARATOR.exec(query) ?? HYPHEN_SEPARATOR.exec(query) ?? DO_SEPARATOR.exec(query)
    if (!match) return null
    const from = match[1].trim()
    const to = match[2].trim()
    if (from.length < 2 || to.length < 2) return null
    return { from, to }
}

/** One end of a connection search: `query` goes to `/schedules/search` as `from` / `to`. */
export interface ConnectionEnd {
    query: string
    label: string
}

/**
 * The api takes a station id or a city / station-name prefix. An exact station name pins that
 * station; anything else ("Warszawa") is passed as text so every station of the city counts.
 * Undefined when no station matches at all.
 */
export function resolveConnectionEnd(suggestions: StationSuggestion[], typed: string): ConnectionEnd | undefined {
    if (suggestions.length === 0) return undefined
    const q = normalizeText(typed)
    const exact = suggestions.find((s) => normalizeText(s.name) === q)
    if (exact) return { query: String(exact.external_id), label: exact.name }
    const text = typed.trim()
    return { query: text, label: text }
}

function trainRank(train: TrainMapPoint, q: string): number | null {
    const name = normalizeText(train.train_name)
    if (name.startsWith(q)) return 0

    const carrier = normalizeText(train.carrier_code ?? '')
    const combined = carrier ? `${carrier} ${name}` : name
    if (name.includes(q) || combined.includes(q) || name.split(' ').some((word) => word.startsWith(q))) return 1

    const others = [carrier, normalizeText(train.origin ?? ''), normalizeText(train.destination ?? '')]
    if (others.some((field) => field.includes(q))) return 2
    return null
}

/** Trains matching the query, best first: name prefix, then name match, then carrier/route. */
export function matchTrains(trains: TrainMapPoint[], q: string, limit = 5): TrainMapPoint[] {
    const query = normalizeText(q)
    if (!query) return []
    return trains
        .map((train) => ({ train, rank: trainRank(train, query) }))
        .filter((entry): entry is { train: TrainMapPoint; rank: number } => entry.rank !== null)
        .sort((a, b) => a.rank - b.rank || a.train.train_name.localeCompare(b.train.train_name, 'pl'))
        .slice(0, limit)
        .map((entry) => entry.train)
}
