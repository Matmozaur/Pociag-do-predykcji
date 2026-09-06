# services/frontend

Next.js 15 (App Router) + React 19 + TypeScript strict + Tailwind v4. See repo root `CLAUDE.md`
for architecture. The frontend talks **only to the gateway** — never collector or data-service.

## Commands (from `services/frontend/`)

```bash
npm install
npm run dev       # next dev on :3000
npm run build     # next build — the ONLY build/lint/typecheck gate
npm start         # next start (prod)
```

- **There is no `lint` or `test` npm script and no standalone ESLint config file.** Linting is
  whatever `next build` runs via `eslint-config-next`; type checking is `tsc` through the build
  (`noEmit`, `strict`). "Run build before finishing" is the check.
- No test runner is set up — don't assume `npm test` exists.

## Talking to the gateway

- `src/lib/api.ts` `apiFetch<T>()` is the single entry point. `BASE` resolves to:
  - browser: `process.env.NEXT_PUBLIC_GATEWAY_BASE ?? '/bff'` — the `/bff/*` path is rewritten to
    the gateway in `next.config.ts` (`GATEWAY_URL ?? 'http://localhost:8084'`).
  - server (RSC): `process.env.GATEWAY_URL ?? 'http://localhost:8084'` directly.
- Responses are cached with `next: { revalidate: 30 }`. Match shapes to
  `specs/openapi/gateway.yml`; declare response `interface`s in `api.ts` (see `StationSuggestion`
  etc.) — no implicit `any`.

## Env vars

`GATEWAY_URL` (server + rewrite target, default `http://localhost:8084`),
`NEXT_PUBLIC_GATEWAY_BASE` (optional browser override of the `/bff` prefix).

## Conventions

- Server data only through `@tanstack/react-query` hooks in `src/lib/` — no ad-hoc `fetch` in
  components. Query client is set up in `src/app/providers.tsx`. Handle loading + error states.
- Route segments in `src/app/` (Polish slugs: `mapa`, `pociagi`, `rozklad`, `utrudnienia`,
  `wyszukaj`); shared components in `src/components/`; utilities/API in `src/lib/`.
- Maps: `react-leaflet` / `leaflet`, in `*Client.tsx` components (client-only). Classes composed
  with `clsx` + `tailwind-merge`. Icons from `lucide-react`. Import alias `@/*` → `src/*`.
