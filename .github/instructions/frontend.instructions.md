---
description: "Frontend conventions for the Next.js app."
applyTo: "services/frontend/**/*.{ts,tsx}"
---

# Frontend Instructions

These rules apply automatically to all TypeScript/TSX files in `services/frontend/`.

## Stack

- Next.js 15 (App Router) + React 19 + TypeScript (strict).
- Data fetching via `@tanstack/react-query` — no ad-hoc `fetch` in components for
  server data; wrap calls in query/mutation hooks under `src/lib/`.
- Maps via `react-leaflet` / `leaflet`.
- Styling with Tailwind CSS v4; compose classes with `clsx` + `tailwind-merge`.
- Icons from `lucide-react`.

## Conventions

- Keep components in `src/components/`, route segments in `src/app/`, and shared
  utilities/API clients in `src/lib/`.
- Type all props and API responses explicitly; no implicit `any`.
- The frontend talks only to the **gateway** BFF — never call collector or
  data-service directly. Match request/response shapes to `specs/openapi/gateway.yml`.
- Handle loading and error states for every remote query.
- No secrets or API tokens in client code; use server components / route handlers for
  privileged calls.

## Quality

- Run `npm run build` and `eslint` (via `eslint-config-next`) before finishing.
- Prefer accessible, semantic markup and keyboard-navigable interactions.
