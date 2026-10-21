# services/frontend

Next.js 15 (App Router) + React 19 + TypeScript strict + Tailwind v4. See repo root `CLAUDE.md`
for architecture. The frontend talks **only to the api** (`services/go/api`).

## Commands (from `services/frontend/`)

```bash
npm install
npm run dev       # next dev on :3000
npm run build     # next build — the ONLY build/lint/typecheck gate
npm start         # next start (prod)
```

- **There is no `lint` or `test` npm script.** `next build` runs ESLint (flat config
  `eslint.config.mjs`: `next/core-web-vitals` + `next/typescript`) and `tsc` (`noEmit`,
  `strict`). "Run build before finishing" is the check; `node_modules/.bin/eslint <path>` lints
  a single file.
- No test runner is set up — don't assume `npm test` exists.

## Talking to the api

- `src/lib/api.ts` `apiFetch<T>()` (used by the `api` object) is the single entry point. `BASE` resolves to:
  - browser: `process.env.NEXT_PUBLIC_API_BASE ?? '/bff'` — the `/bff/*` path is rewritten to
    the api in `next.config.ts` (`API_URL ?? 'http://localhost:8080'`).
  - server (RSC): `process.env.API_URL ?? 'http://localhost:8080'` directly.
- All fetching happens in the browser, so `apiFetch`'s `next: { revalidate: 30 }` has no
  effect; freshness comes from TanStack Query. Match shapes to `specs/openapi/api.yml`; declare response `interface`s in `api.ts` (see `StationSuggestion`
  etc.) — no implicit `any`.

## Env vars

`API_URL` (server + rewrite target, default `http://localhost:8080`),
`NEXT_PUBLIC_API_BASE` (optional browser override of the `/bff` prefix).

## Conventions

- Server data only through `@tanstack/react-query` `useQuery` calling the `api` object — inline
  in the page/component, or a shared hook in `src/lib/` when reused (`useStationBoard.ts`). No
  ad-hoc `fetch` to the api in components (static files from `public/`, e.g.
  `poland-border.geojson`, are the exception). Query client (default `staleTime` 30 s) is set
  up in `src/app/providers.tsx`. Handle loading + error states.
- Route segments in `src/app/`: Polish slugs `mapa` (home; `/` redirects there), `pociagi`,
  `pociagi/[id]`, `rozklad/[id]`, `utrudnienia`, `wyszukaj`, plus `stations/[id]` (station
  board). Pages are client components (`'use client'`) wrapped in `NavShell`.
- Shared components in `src/components/`; UI primitives (`cn`, `Badge`, `Button`, `Card`,
  `Input`, `Spinner`, `EmptyState`) in `src/lib/ui.tsx`; API client and formatting helpers
  (`formatDelay`, `statusLabel`, …) in `src/lib/api.ts`.
- Maps: `react-leaflet` / `leaflet`, in `*Client.tsx` components (client-only). Classes composed
  with `clsx` + `tailwind-merge`. Icons from `lucide-react`. Import alias `@/*` → `src/*`.
