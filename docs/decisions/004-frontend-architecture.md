# ADR-004 — Frontend Architecture: Pociąg do Predykcji Web UI

**Status**: Accepted  
**Date**: 2026-07-06  
**Author**: Architect agent

---

## Context

The backend BFF gateway (`services/go/gateway`) exposes a stable REST API on `http://localhost:8080`. The frontend must present five pages in Polish targeting a dark, transit-dashboard aesthetic. No authentication is required. Stations have no lat/lng coordinates in the schema, so map geometry must come from a static hardcoded lookup inside the frontend.

---

## Decision

Build a **Next.js 15 App Router** application at `services/frontend/`. All five pages are React Server Components at the route level with targeted Client Component islands. API calls to the gateway are abstracted through a typed fetch client wrapped in TanStack Query hooks. The map uses React Leaflet with OpenRailwayMap overlay tiles and a local mock GeoJSON endpoint for traffic volume polylines.

---

## 1. Technology Decisions

| Concern | Choice | Rejected Alternatives | Rationale |
|---|---|---|---|
| Framework | **Next.js 15 App Router** | Vite + React SPA, Remix | RSC for schedule/disruption pages (SEO, TTFB); nested layouts for AppShell without prop drilling; built-in Route Handlers for mock API |
| Styling | **Tailwind CSS v4** | CSS Modules, styled-components | Design tokens as CSS custom properties; dark mode via `dark:` classes; no runtime overhead |
| Component primitives | **shadcn/ui** (Radix-based) | MUI, Ant Design | Components live in `src/components/ui/` — owned, not dependency-locked; full Tailwind integration; accessible by default |
| Data fetching | **TanStack Query v5** | SWR, RTK Query | Per-query stale times; polling (`refetchInterval`) for live train data; devtools for debugging; consistent loading/error states |
| Map | **react-leaflet v4** | MapLibre GL, deck.gl | Mature L.Polyline API needed for colored line segments; OpenRailwayMap tiles are Leaflet-native; lighter bundle than WebGL options |
| Forms | **react-hook-form v7** | Formik | Uncontrolled inputs, minimal re-renders; integrates cleanly with shadcn/ui `<Input>` |
| Date input | **react-day-picker v9** | Flatpickr, native `<input type=date>` | Headless, styled via Tailwind; shadcn/ui `Calendar` component wraps it |
| Package manager | **pnpm** | npm, yarn | Workspace support for potential monorepo growth; disk-efficient |
| Type checking | **TypeScript 5 strict** | — | Project convention; gateway types generated from spec manually |

---

## 2. Directory Structure

```
services/frontend/
├── src/
│   ├── app/                              # Next.js App Router
│   │   ├── layout.tsx                   # Root layout: QueryProvider, AppShell
│   │   ├── page.tsx                     # /  → Mapa
│   │   ├── wyszukaj/
│   │   │   └── page.tsx                 # /wyszukaj → Wyszukaj połączeń
│   │   ├── pociag/
│   │   │   └── [id]/
│   │   │       └── page.tsx             # /pociag/[id] → Detail pociągu
│   │   ├── utrudnienia/
│   │   │   └── page.tsx                 # /utrudnienia → Lista utrudnień
│   │   ├── rozklad/
│   │   │   └── [id]/
│   │   │       └── page.tsx             # /rozklad/[id] → Rozkład trasy
│   │   └── api/
│   │       └── mock/
│   │           └── traffic/
│   │               └── route.ts         # GET /api/mock/traffic → GeoJSON
│   │
│   ├── components/
│   │   ├── layout/
│   │   │   ├── AppShell.tsx             # Sidebar + main wrapper
│   │   │   ├── SideNav.tsx              # Desktop: 240px left sidebar
│   │   │   ├── BottomNav.tsx            # Mobile: bottom bar (5 items)
│   │   │   ├── TopBar.tsx               # Mobile: title + action slot
│   │   │   └── NavItem.tsx              # Shared nav link (icon + label)
│   │   │
│   │   ├── map/
│   │   │   ├── RailwayMap.tsx           # 'use client' Leaflet root
│   │   │   ├── TrafficLayer.tsx         # Polylines colored by volume
│   │   │   ├── StationDot.tsx           # CircleMarker at city coords
│   │   │   ├── MapTooltip.tsx           # Leaflet Tooltip wrapper
│   │   │   ├── MapLegend.tsx            # Volume color scale (corner overlay)
│   │   │   └── DashboardOverlay.tsx     # Stat cards floating top-right
│   │   │
│   │   ├── search/
│   │   │   ├── ConnectionSearchForm.tsx # Form state owner ('use client')
│   │   │   ├── StationAutocomplete.tsx  # Combobox + debounced query
│   │   │   ├── ConnectionResultCard.tsx # Single result row
│   │   │   └── SearchResultsList.tsx    # Renders cards + pagination
│   │   │
│   │   ├── train/
│   │   │   ├── TrainHeader.tsx          # Name, carrier, status, delay
│   │   │   ├── JourneyTimeline.tsx      # Scrollable stop list
│   │   │   ├── StopRow.tsx              # Single stop (planned vs actual)
│   │   │   ├── DelayBadge.tsx           # Colored +Xmin chip
│   │   │   └── StatusBadge.tsx          # in_progress / completed / cancelled
│   │   │
│   │   ├── disruptions/
│   │   │   ├── DisruptionCard.tsx       # Card with severity stripe
│   │   │   ├── SeverityBadge.tsx        # low / medium / high pill
│   │   │   └── DisruptionBanner.tsx     # Compact banner (used in map overlay)
│   │   │
│   │   ├── schedule/
│   │   │   ├── ScheduleHeader.tsx       # Train info + duration
│   │   │   ├── OperatingCalendar.tsx    # Mini calendar of operating dates
│   │   │   └── ScheduleStopRow.tsx      # Timetable row (arr/dep/platform)
│   │   │
│   │   ├── dashboard/
│   │   │   ├── StatCard.tsx             # Single KPI tile
│   │   │   └── FreshnessLabel.tsx       # "Dane z: 14:23" timestamp
│   │   │
│   │   └── ui/                          # shadcn/ui primitives (generated)
│   │       ├── button.tsx
│   │       ├── input.tsx
│   │       ├── badge.tsx
│   │       ├── card.tsx
│   │       ├── popover.tsx
│   │       ├── command.tsx              # Combobox base
│   │       ├── calendar.tsx
│   │       ├── skeleton.tsx
│   │       ├── separator.tsx
│   │       └── tooltip.tsx
│   │
│   ├── lib/
│   │   ├── api/
│   │   │   ├── client.ts                # Base fetch + ApiError
│   │   │   ├── gateway.ts               # Named functions per endpoint
│   │   │   └── types.ts                 # TypeScript interfaces from spec
│   │   │
│   │   ├── hooks/                       # TanStack Query wrappers
│   │   │   ├── useStationSearch.ts      # searchStations (debounced)
│   │   │   ├── useScheduleSearch.ts     # searchSchedules
│   │   │   ├── useScheduleDetail.ts     # getScheduleDetail
│   │   │   ├── useLiveTrains.ts         # getLiveTrains (polling 60s)
│   │   │   ├── useTrainDetail.ts        # getTrainDetail (polling 60s)
│   │   │   ├── useDisruptions.ts        # listDisruptions
│   │   │   ├── useDashboard.ts          # getDashboardOverview (polling 120s)
│   │   │   └── useTrafficData.ts        # /api/mock/traffic (static, long TTL)
│   │   │
│   │   ├── mock/
│   │   │   └── trafficGeoJson.ts        # GeoJSON FeatureCollection builder
│   │   │
│   │   └── utils/
│   │       ├── delay.ts                 # delayToColor, delayLabel
│   │       ├── status.ts                # statusToLabel, statusToColor (Polish)
│   │       ├── carriers.ts              # Carrier → badge color map
│   │       └── formatters.ts            # formatTime, formatDuration (Polish)
│   │
│   └── styles/
│       └── globals.css                  # Tailwind directives + CSS vars
│
├── public/
│   └── favicon.ico
├── next.config.ts
├── tailwind.config.ts
├── components.json                      # shadcn/ui CLI config
├── tsconfig.json
├── package.json
└── .env.local.example
```

---

## 3. Component Hierarchy per Page

### 3.1 Mapa (`/`)

```
RootLayout
└── AppShell
    ├── SideNav (desktop) | BottomNav (mobile)
    └── main
        ├── RailwayMap                       [client, fills viewport]
        │   ├── TileLayer: CartoDB Dark Matter
        │   ├── TileLayer: OpenRailwayMap (standard)
        │   ├── TrafficLayer
        │   │   └── Polyline × 25           [volume-colored]
        │   ├── StationDot × 20             [CircleMarker per city]
        │   └── MapLegend                   [absolute, bottom-left]
        └── DashboardOverlay                [absolute, top-right, z-[1000]]
            ├── StatCard "Pociągów dziś"
            ├── StatCard "Na czas"
            ├── StatCard "Opóźnione"
            └── DisruptionBanner            [link to /utrudnienia]
```

> **Constraint**: `RailwayMap` must be a `'use client'` component imported via `dynamic(() => import(...), { ssr: false })` to prevent Leaflet SSR crash. `DashboardOverlay` is a separate RSC-compatible component placed outside the map.

### 3.2 Wyszukaj (`/wyszukaj`)

```
WyszukajPage (RSC — reads searchParams for URL-state)
└── AppShell
    └── main
        ├── PageHeader "Szukaj połączeń"
        ├── ConnectionSearchForm            [client]
        │   ├── StationAutocomplete "Skąd" → useStationSearch (debounce 300ms)
        │   │   └── Command + CommandItem×N
        │   ├── SwapStationsButton          [client, swaps form values]
        │   ├── StationAutocomplete "Dokąd"
        │   └── DatePicker                 [Calendar popover]
        └── SearchResultsList              [client, conditional on params]
            ├── ResultsHeader (N połączeń, sortowanie)
            ├── ConnectionResultCard × N
            │   ├── CarrierBadge
            │   ├── TimeRange (HH:MM → HH:MM)
            │   ├── DurationChip
            │   ├── StopsCount
            │   └── Link → /rozklad/[route_id]
            └── Pagination
```

### 3.3 Pociąg (`/pociag/[id]`)

```
PociagPage (RSC — [id] from params)
└── AppShell
    └── main
        ├── TopBar (mobile): back button + "Pociąg [name]"
        └── TrainDetailContent             [client — polling]
            ├── TrainHeader
            │   ├── CarrierBadge
            │   ├── train_name (h1)
            │   ├── StatusBadge
            │   ├── DelayBadge (current max delay)
            │   └── FreshnessLabel
            ├── JourneyTimeline
            │   └── StopRow × N
            │       ├── sequence indicator (dot + line)
            │       ├── station_name
            │       ├── PlannedTime (arrival / departure)
            │       ├── ActualTime (conditional, coloured)
            │       ├── DelayBadge (per-stop, conditional)
            │       └── ConfirmedIcon | CancelledStrikethrough
            └── RefreshIndicator ("Odświeża co 60s")
```

### 3.4 Utrudnienia (`/utrudnienia`)

```
UtrudnieniaPage (RSC — can be fully server-rendered)
└── AppShell
    └── main
        ├── PageHeader "Utrudnienia" + active-count Badge
        ├── FilterBar                      [client]
        │   ├── SeverityFilter (checkboxes: niski/średni/wysoki)
        │   └── ActiveOnlyToggle
        └── DisruptionList
            └── DisruptionCard × N
                ├── SeverityBadge          [left border colour stripe]
                ├── RouteSpan "StacjaA → StacjaB"
                ├── DateRange "dd.MM – dd.MM"
                ├── MessageText (truncated, expandable)
                └── AffectedRoutesBadge "X tras"
```

### 3.5 Rozkład (`/rozklad/[id]`)

```
RozkladPage (RSC)
└── AppShell
    └── main
        ├── ScheduleHeader
        │   ├── CarrierBadge
        │   ├── CommercialCategoryBadge (IC / TLK / REG)
        │   ├── train_name (h1)
        │   └── TotalDurationChip
        ├── OperatingCalendar              [client, react-day-picker, read-only]
        └── StopList
            └── ScheduleStopRow × N
                ├── order indicator
                ├── station_name
                ├── arrival_time
                ├── departure_time
                ├── PlatformBadge (conditional)
                └── StopTypePill (conditional: "Postój techniczny")
```

---

## 4. API Client Design

### 4.1 `lib/api/types.ts`

TypeScript interfaces that mirror `specs/openapi/gateway.yml` component schemas **exactly** — names are preserved as-is from the spec. Additions are only permitted for frontend convenience (e.g., a `delayLevel` computed field added by a transformer, not the raw type).

Key types:

```typescript
// Mirrors StationSuggestion schema
export interface StationSuggestion {
  external_id: number;
  name: string;
  city?: string;
}

// Mirrors LiveTrainSummary
export interface LiveTrainSummary {
  operation_id: number;
  train_name: string;
  carrier_code?: string;
  status: "not_started" | "in_progress" | "completed" | "cancelled" | "partial_cancelled";
  status_code?: string;
  current_station?: string;
  next_station?: string;
  delay_minutes?: number;
  origin?: string;
  destination?: string;
}

// Mirrors TrainStopView
export interface TrainStopView {
  station_name: string;
  station_external_id?: number;
  sequence: number;
  planned_arrival?: string;
  planned_departure?: string;
  actual_arrival?: string;
  actual_departure?: string;
  arrival_delay_minutes?: number;
  departure_delay_minutes?: number;
  is_confirmed: boolean;
  is_cancelled: boolean;
}
// ... (all other schemas)
```

### 4.2 `lib/api/client.ts`

```typescript
const GATEWAY_URL =
  process.env.NEXT_PUBLIC_GATEWAY_URL ?? "http://localhost:8080";

export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export async function apiFetch<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const url = `${GATEWAY_URL}${path}`;
  const res = await fetch(url, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: res.statusText }));
    throw new ApiError(res.status, body.message ?? res.statusText);
  }
  return res.json() as Promise<T>;
}
```

> **Security note**: `NEXT_PUBLIC_GATEWAY_URL` is the only configurable value. Never embed secrets in `NEXT_PUBLIC_*` vars.

### 4.3 `lib/api/gateway.ts`

```typescript
import { apiFetch } from "./client";
import type {
  StationSuggestionsResponse,
  ScheduleSearchResponse,
  ScheduleDetailView,
  LiveTrainsResponse,
  TrainDetailView,
  DisruptionListView,
  DashboardOverview,
} from "./types";

export interface ScheduleSearchParams {
  from: string;
  to: string;
  date: string;           // YYYY-MM-DD
  carriers?: string;
  categories?: string;
  sort?: "departure" | "arrival" | "duration";
  limit?: number;
  offset?: number;
}

export interface LiveTrainsParams {
  carriers?: string;
  stations?: string;
  limit?: number;
  offset?: number;
}

export interface DisruptionsParams {
  active?: boolean;
  limit?: number;
  offset?: number;
}

function qs(params: Record<string, unknown>): string {
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== null) p.set(k, String(v));
  }
  const s = p.toString();
  return s ? `?${s}` : "";
}

export const gateway = {
  searchStations: (q: string, limit = 10) =>
    apiFetch<StationSuggestionsResponse>(
      `/api/v1/search/stations?q=${encodeURIComponent(q)}&limit=${limit}`,
    ),

  searchSchedules: (params: ScheduleSearchParams) =>
    apiFetch<ScheduleSearchResponse>(`/api/v1/schedules/search${qs(params)}`),

  getScheduleDetail: (routeId: number) =>
    apiFetch<ScheduleDetailView>(`/api/v1/schedules/${routeId}`),

  getLiveTrains: (params?: LiveTrainsParams) =>
    apiFetch<LiveTrainsResponse>(`/api/v1/trains/live${qs(params ?? {})}`),

  getTrainDetail: (operationId: number) =>
    apiFetch<TrainDetailView>(`/api/v1/trains/${operationId}`),

  listDisruptions: (params?: DisruptionsParams) =>
    apiFetch<DisruptionListView>(`/api/v1/disruptions${qs(params ?? {})}`),

  getDashboardOverview: () =>
    apiFetch<DashboardOverview>(`/api/v1/dashboard/overview`),
};
```

### 4.4 TanStack Query Hooks — `lib/hooks/`

Each hook has a canonical `queryKey` shape and appropriate `staleTime`/`refetchInterval`:

| Hook | queryKey | staleTime | refetchInterval |
|---|---|---|---|
| `useStationSearch(q)` | `["stations", q]` | 5 min | — |
| `useScheduleSearch(params)` | `["schedules", "search", params]` | 2 min | — |
| `useScheduleDetail(id)` | `["schedules", id]` | 10 min | — |
| `useLiveTrains(params)` | `["trains", "live", params]` | 30 s | 60 000 ms |
| `useTrainDetail(id)` | `["trains", id]` | 30 s | 60 000 ms |
| `useDisruptions(params)` | `["disruptions", params]` | 2 min | — |
| `useDashboard()` | `["dashboard"]` | 1 min | 120 000 ms |
| `useTrafficData()` | `["mock", "traffic"]` | ∞ (static) | — |

`useStationSearch` gates the query with `enabled: q.length >= 2` and wraps with a 300ms debounce inside the component via `useDeferredValue` or a `useDebounce` utility.

### 4.5 Mock Traffic Route Handler — `app/api/mock/traffic/route.ts`

```typescript
import { NextResponse } from "next/server";
import { buildTrafficGeoJson } from "@/lib/mock/trafficGeoJson";

export const dynamic = "force-static"; // built at build time, no runtime cost

export function GET(): NextResponse {
  return NextResponse.json(buildTrafficGeoJson());
}
```

`buildTrafficGeoJson()` in `lib/mock/trafficGeoJson.ts` assembles the FeatureCollection at import time (see §6 below).

---

## 5. Color & Theme System

### 5.1 CSS Custom Properties — `styles/globals.css`

```css
@import "tailwindcss";

:root {
  /* ── Surfaces ── */
  --bg-base:      #0d1117;
  --bg-surface:   #161b22;
  --bg-elevated:  #21262d;
  --bg-hover:     #292f38;
  --border:       #30363d;
  --border-dim:   #21262d;

  /* ── Text ── */
  --text-primary:   #e6edf3;
  --text-secondary: #8b949e;
  --text-muted:     #484f58;

  /* ── Brand accent ── */
  --brand:     #388bfd;
  --brand-dim: #1f6feb;

  /* ── Train status ── */
  --status-ok:        #3fb950;   /* ≤1 min delay or completed on time */
  --status-warn:      #d29922;   /* 2–14 min delay */
  --status-late:      #f85149;   /* ≥15 min delay */
  --status-cancelled: #6e7681;   /* X or Q */
  --status-progress:  #388bfd;   /* in_progress */
  --status-pending:   #8b949e;   /* not_started */

  /* ── Traffic volume scale (5 stops) ── */
  --vol-low:     #3b82f6;   /* < 200 trains/day */
  --vol-normal:  #22c55e;   /* 200–400 */
  --vol-busy:    #eab308;   /* 400–600 */
  --vol-heavy:   #f97316;   /* 600–800 */
  --vol-peak:    #ef4444;   /* > 800 */

  /* ── Severity ── */
  --sev-low:    #3b82f6;
  --sev-medium: #f97316;
  --sev-high:   #f85149;
}
```

### 5.2 Tailwind Configuration — `tailwind.config.ts`

```typescript
import type { Config } from "tailwindcss";

export default {
  darkMode: "class",             // always add "dark" to <html>
  content: ["./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        base:      "var(--bg-base)",
        surface:   "var(--bg-surface)",
        elevated:  "var(--bg-elevated)",
        brand:     "var(--brand)",
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
        mono: ["JetBrains Mono", "monospace"],
      },
    },
  },
} satisfies Config;
```

### 5.3 Delay Color Function — `lib/utils/delay.ts`

```typescript
export type DelayLevel = "on-time" | "warn" | "late" | "unknown";

export function delayLevel(minutes: number | null | undefined): DelayLevel {
  if (minutes === null || minutes === undefined) return "unknown";
  if (minutes <= 1) return "on-time";
  if (minutes <= 14) return "warn";
  return "late";
}

export const DELAY_COLORS: Record<DelayLevel, string> = {
  "on-time": "var(--status-ok)",
  warn:      "var(--status-warn)",
  late:      "var(--status-late)",
  unknown:   "var(--text-muted)",
};

export const DELAY_TAILWIND: Record<DelayLevel, string> = {
  "on-time": "text-green-400",
  warn:      "text-amber-400",
  late:      "text-red-400",
  unknown:   "text-zinc-500",
};
```

### 5.4 Status Labels (Polish) — `lib/utils/status.ts`

```typescript
export const STATUS_LABELS: Record<string, string> = {
  not_started:      "Nie rozpoczął",
  in_progress:      "W drodze",
  completed:        "Zakończył kurs",
  cancelled:        "Odwołany",
  partial_cancelled:"Częściowo odwołany",
};

export const STATUS_TAILWIND: Record<string, string> = {
  not_started:      "bg-zinc-700 text-zinc-300",
  in_progress:      "bg-blue-900 text-blue-300",
  completed:        "bg-green-900 text-green-300",
  cancelled:        "bg-red-950 text-red-400 line-through",
  partial_cancelled:"bg-yellow-900 text-yellow-300",
};
```

### 5.5 Carrier Badge Colors — `lib/utils/carriers.ts`

```typescript
export interface CarrierStyle {
  bg: string;    // Tailwind bg class
  text: string;  // Tailwind text class
  label: string; // Full carrier name
}

export const CARRIER_STYLES: Record<string, CarrierStyle> = {
  IC:  { bg: "bg-red-700",    text: "text-white",     label: "PKP Intercity" },
  EIC: { bg: "bg-red-800",    text: "text-white",     label: "PKP Intercity (EIC)" },
  TLK: { bg: "bg-orange-700", text: "text-white",     label: "PKP Intercity (TLK)" },
  KM:  { bg: "bg-blue-700",   text: "text-white",     label: "Koleje Mazowieckie" },
  KS:  { bg: "bg-orange-600", text: "text-white",     label: "Koleje Śląskie" },
  PR:  { bg: "bg-purple-700", text: "text-white",     label: "Polregio" },
  ŁKA: { bg: "bg-red-600",    text: "text-white",     label: "Łódzka KA" },
  WKD: { bg: "bg-green-700",  text: "text-white",     label: "WKD" },
  SKM: { bg: "bg-cyan-700",   text: "text-white",     label: "SKM Trójmiasto" },
};

export function carrierStyle(code: string | undefined): CarrierStyle {
  return (
    CARRIER_STYLES[code?.toUpperCase() ?? ""] ?? {
      bg: "bg-zinc-700",
      text: "text-zinc-300",
      label: code ?? "Nieznany",
    }
  );
}
```

### 5.6 Traffic Volume → Polyline Color — `lib/utils/trafficColor.ts`

```typescript
// 5-stop linear interpolation over volume range 0–1000
const STOPS: [number, string][] = [
  [0,    "#3b82f6"],  // blue
  [250,  "#22c55e"],  // green
  [500,  "#eab308"],  // yellow
  [750,  "#f97316"],  // orange
  [1000, "#ef4444"],  // red
];

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

function hexToRgb(hex: string): [number, number, number] {
  const n = parseInt(hex.slice(1), 16);
  return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

function rgbToHex(r: number, g: number, b: number): string {
  return `#${[r, g, b].map((v) => Math.round(v).toString(16).padStart(2, "0")).join("")}`;
}

export function volumeToColor(volume: number): string {
  const clamped = Math.max(0, Math.min(volume, 1000));
  for (let i = 0; i < STOPS.length - 1; i++) {
    const [v0, c0] = STOPS[i];
    const [v1, c1] = STOPS[i + 1];
    if (clamped <= v1) {
      const t = (clamped - v0) / (v1 - v0);
      const [r0, g0, b0] = hexToRgb(c0);
      const [r1, g1, b1] = hexToRgb(c1);
      return rgbToHex(lerp(r0, r1, t), lerp(g0, g1, t), lerp(b0, b1, t));
    }
  }
  return "#ef4444";
}
```

---

## 6. Mock Traffic GeoJSON API

### 6.1 Station Coordinate Lookup — `lib/mock/trafficGeoJson.ts`

```typescript
// [lat, lng] WGS84 — hardcoded, no external dependency
const CITY_COORDS: Record<string, [number, number]> = {
  Warszawa:     [52.2297, 21.0122],
  Kraków:       [50.0647, 19.9450],
  Gdańsk:       [54.3520, 18.6466],
  Wrocław:      [51.1079, 17.0385],
  Poznań:       [52.4064, 16.9252],
  Łódź:         [51.7592, 19.4560],
  Katowice:     [50.2649, 19.0238],
  Szczecin:     [53.4285, 14.5528],
  Lublin:       [51.2465, 22.5684],
  Białystok:    [53.1325, 23.1688],
  Rzeszów:      [50.0412, 21.9991],
  Bydgoszcz:    [53.1235, 18.0084],
  Toruń:        [53.0137, 18.5981],
  Kielce:       [50.8661, 20.6286],
  Radom:        [51.4027, 21.1471],
  Olsztyn:      [53.7784, 20.4801],
  Opole:        [50.6751, 17.9213],
  Zielona_Góra: [51.9356, 15.5062],
  Częstochowa:  [50.8118, 19.1203],
  Gliwice:      [50.2945, 18.6714],
};
```

### 6.2 Rail Line Definitions

```typescript
interface RailLine {
  from: string;
  to: string;
  volume: number;   // trains/day (mock)
  line_name: string;
}

const RAIL_LINES: RailLine[] = [
  { from: "Warszawa",     to: "Kraków",       volume: 920, line_name: "CMK" },
  { from: "Warszawa",     to: "Gdańsk",       volume: 780, line_name: "E65" },
  { from: "Warszawa",     to: "Łódź",         volume: 650, line_name: "E20-W" },
  { from: "Warszawa",     to: "Wrocław",      volume: 540, line_name: "E30-W" },
  { from: "Warszawa",     to: "Lublin",       volume: 420, line_name: "Wschodnia" },
  { from: "Warszawa",     to: "Białystok",    volume: 310, line_name: "E75-N" },
  { from: "Warszawa",     to: "Radom",        volume: 510, line_name: "E65-S" },
  { from: "Kraków",       to: "Katowice",     volume: 840, line_name: "E30" },
  { from: "Kraków",       to: "Wrocław",      volume: 480, line_name: "E30-W" },
  { from: "Kraków",       to: "Rzeszów",      volume: 560, line_name: "E30-E" },
  { from: "Kraków",       to: "Kielce",       volume: 310, line_name: "E65-S" },
  { from: "Gdańsk",       to: "Bydgoszcz",    volume: 610, line_name: "CE65" },
  { from: "Gdańsk",       to: "Szczecin",     volume: 290, line_name: "CE59-N" },
  { from: "Gdańsk",       to: "Olsztyn",      volume: 270, line_name: "E65-NE" },
  { from: "Wrocław",      to: "Poznań",       volume: 450, line_name: "E59" },
  { from: "Wrocław",      to: "Katowice",     volume: 720, line_name: "E30" },
  { from: "Wrocław",      to: "Opole",        volume: 580, line_name: "E30-W2" },
  { from: "Wrocław",      to: "Zielona_Góra", volume: 240, line_name: "CE59-W" },
  { from: "Poznań",       to: "Bydgoszcz",    volume: 380, line_name: "CE59" },
  { from: "Poznań",       to: "Szczecin",     volume: 310, line_name: "CE59-W2" },
  { from: "Katowice",     to: "Gliwice",      volume: 700, line_name: "Śląska" },
  { from: "Katowice",     to: "Częstochowa",  volume: 430, line_name: "CE65-S" },
  { from: "Łódź",         to: "Katowice",     volume: 390, line_name: "ŁKA" },
  { from: "Bydgoszcz",    to: "Toruń",        volume: 460, line_name: "CE65-C" },
  { from: "Lublin",       to: "Rzeszów",      volume: 220, line_name: "Wschodnia-S" },
];
```

### 6.3 GeoJSON Shape

Each rail line becomes one `Feature<LineString>`:

```typescript
import type { FeatureCollection, Feature, LineString } from "geojson";

export interface TrafficProperties {
  line_name: string;
  from_city: string;
  to_city: string;
  volume: number;          // trains/day
  volume_label: string;    // human readable "920 poc./dzień"
}

export function buildTrafficGeoJson(): FeatureCollection<LineString, TrafficProperties> {
  const features: Feature<LineString, TrafficProperties>[] = RAIL_LINES
    .filter((l) => CITY_COORDS[l.from] && CITY_COORDS[l.to])
    .map((l) => {
      const [lat1, lng1] = CITY_COORDS[l.from];
      const [lat2, lng2] = CITY_COORDS[l.to];
      return {
        type: "Feature",
        geometry: {
          type: "LineString",
          // GeoJSON uses [lng, lat] order
          coordinates: [[lng1, lat1], [lng2, lat2]],
        },
        properties: {
          line_name:    l.line_name,
          from_city:    l.from,
          to_city:      l.to,
          volume:       l.volume,
          volume_label: `${l.volume} poc./dzień`,
        },
      };
    });

  return { type: "FeatureCollection", features };
}
```

### 6.4 Leaflet Polyline Rendering — `components/map/TrafficLayer.tsx`

```tsx
"use client";
import { Polyline, Tooltip } from "react-leaflet";
import { volumeToColor } from "@/lib/utils/trafficColor";
import { useTrafficData } from "@/lib/hooks/useTrafficData";

export function TrafficLayer() {
  const { data } = useTrafficData();
  if (!data) return null;

  return (
    <>
      {data.features.map((f, i) => {
        // GeoJSON coords are [lng, lat]; Leaflet wants [lat, lng]
        const positions = f.geometry.coordinates.map(
          ([lng, lat]) => [lat, lng] as [number, number],
        );
        const color = volumeToColor(f.properties.volume);
        return (
          <Polyline
            key={i}
            positions={positions}
            pathOptions={{ color, weight: 4, opacity: 0.85 }}
          >
            <Tooltip sticky>
              <span className="font-mono text-xs">
                {f.properties.line_name}: {f.properties.volume_label}
              </span>
            </Tooltip>
          </Polyline>
        );
      })}
    </>
  );
}
```

> **Note**: The `TrafficLayer` renders straight-line segments between cities, NOT real track geometry. Real geometry comes from the OpenRailwayMap tile overlay. The colored polylines are purely a traffic volume visualisation overlay — they intentionally approximate the corridors.

---

## 7. Navigation Pattern

### 7.1 Structure

```
Nav items (5):
  1. Mapa          /              (MapIcon)
  2. Wyszukaj      /wyszukaj      (SearchIcon)
  3. Na żywo       /              (ActivityIcon) — opens map filtered to live view
  4. Utrudnienia   /utrudnienia   (AlertTriangleIcon + badge)
  5. Pociągi       /              (TrainIcon)    — future: live trains list page
```

Items 3 and 5 are stubs that will get dedicated pages when `GET /api/v1/trains/live` list view is built. For now they both link to `/`.

### 7.2 Desktop: `SideNav`

- Fixed left column, `w-60`, `bg-[var(--bg-surface)]`, full viewport height.
- Logo / project name at top (24px, `text-brand`).
- Nav items as vertical list: icon (20px) + label, `gap-3`, `rounded-md`, active item gets `bg-[var(--bg-elevated)] text-[var(--text-primary)]`.
- Bottom section: data freshness indicator.
- Main content area: `ml-60`, `min-h-screen`, `bg-[var(--bg-base)]`.

### 7.3 Mobile: `BottomNav` + `TopBar`

- `BottomNav`: fixed bottom bar, `h-16`, `bg-[var(--bg-surface)]`, `border-t border-[var(--border)]`. Items are icon + short label (10px), centered vertically.
- `TopBar`: fixed top, `h-14`, back-chevron on detail pages, page title center, action slot right (e.g., refresh button on `/pociag/[id]`).
- Main content: `pt-14 pb-16` padding to clear fixed bars.

### 7.4 Breakpoint Strategy

- `< 768px` (md): BottomNav + TopBar, full-width pages, cards stack vertically.
- `≥ 768px`: SideNav, content spans `calc(100vw - 240px)`.
- Map page: `h-screen` minus nav height, Leaflet fills remaining space.

### 7.5 `AppShell.tsx` Skeleton

```tsx
export function AppShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex h-screen bg-[var(--bg-base)] text-[var(--text-primary)]">
      {/* Desktop sidebar */}
      <SideNav className="hidden md:flex" />

      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Mobile top bar */}
        <TopBar className="flex md:hidden" />

        <main className="flex-1 overflow-auto">
          {children}
        </main>

        {/* Mobile bottom nav */}
        <BottomNav className="flex md:hidden" />
      </div>
    </div>
  );
}
```

---

## 8. `next.config.ts` Key Settings

```typescript
import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Gateway rewrites: avoids exposing GATEWAY_URL to browser in prod
  async rewrites() {
    return [
      {
        source: "/bff/:path*",
        destination: `${process.env.GATEWAY_URL ?? "http://localhost:8080"}/:path*`,
      },
    ];
  },
};

export default nextConfig;
```

With this rewrite, the browser calls `/bff/api/v1/...` and Next.js proxies server-side. `GATEWAY_URL` is a server-only env var; `NEXT_PUBLIC_GATEWAY_URL` is only needed for local dev with the dev server's direct fetch mode.

> **Security**: The rewrite approach prevents CORS issues and avoids leaking the backend URL in the browser's network tab in production.

---

## 9. `.env.local.example`

```bash
# Backend gateway (used by Next.js server-side rewrite — NOT exposed to browser)
GATEWAY_URL=http://localhost:8080

# Optional: direct browser fetch during local dev (skip rewrite)
# NEXT_PUBLIC_GATEWAY_URL=http://localhost:8080
```

---

## 10. `package.json` Key Dependencies

```json
{
  "dependencies": {
    "next": "^15.0.0",
    "react": "^19.0.0",
    "react-dom": "^19.0.0",
    "@tanstack/react-query": "^5.0.0",
    "react-leaflet": "^4.2.1",
    "leaflet": "^1.9.4",
    "react-hook-form": "^7.54.0",
    "react-day-picker": "^9.5.0",
    "lucide-react": "^0.468.0",
    "clsx": "^2.1.1",
    "tailwind-merge": "^2.6.0",
    "@radix-ui/react-popover": "^1.1.0",
    "@radix-ui/react-command": "^1.0.0",
    "geojson": "^0.5.0"
  },
  "devDependencies": {
    "typescript": "^5.7.0",
    "@types/react": "^19.0.0",
    "@types/leaflet": "^1.9.15",
    "@types/geojson": "^7946.0.16",
    "tailwindcss": "^4.0.0",
    "@tailwindcss/postcss": "^4.0.0"
  }
}
```

---

## 11. Future Extensibility Notes

These sections are **not implemented** in the initial build but the architecture accommodates them without breaking changes:

| Future Feature | Hook Point |
|---|---|
| Delay predictions | Add `GET /api/v1/trains/{id}/prediction` → new `usePrediction(id)` hook; add `PredictionBadge` alongside `DelayBadge` in `StopRow` |
| Route statistics | Add `/statystyki/[stationId]` page; reuse `StatCard` component |
| Train-specific stats | Add stats tab to `/pociag/[id]` — TanStack Query with same `operationId` key |
| Real-time via WebSocket | Replace `refetchInterval` in `useLiveTrains` / `useTrainDetail` with a WebSocket listener; no component changes needed |
| Map station markers from API | `StationDot` already accepts `[lat, lng]` props; swap hardcoded coords for API-resolved ones when backend provides coordinates |

---

## Consequences

**Positive**:
- Five pages deliverable independently (each page is isolated RSC + client island).
- TanStack Query cache prevents duplicate in-flight requests across components.
- Next.js rewrite centralises CORS handling — no gateway config changes needed.
- `dynamic(..., { ssr: false })` for Leaflet is the established pattern; avoids `window` errors.
- All design tokens in CSS vars → theme-able in one file.

**Negative / Risks**:
- React Leaflet v4 requires careful hydration handling (`dynamic` import is mandatory).
- Traffic polylines are straight lines, not real rail paths; this is acceptable as an overlay on real OpenRailwayMap tiles but must be clearly communicated to users (e.g., tooltip says "przybliżony korytarz").
- `force-static` on the mock traffic route means it is baked at build time; fine for a static mock but must be changed to `dynamic` if mock data is ever runtime-generated.
