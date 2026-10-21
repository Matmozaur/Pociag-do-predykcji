---
mode: agent
description: Design and write an OpenAPI spec for a new service
---

Design an OpenAPI 3.1 spec for a new service.

## Service description

**Name**: `${input:serviceName}`
**Purpose**: `${input:servicePurpose}`
**Language**: `${input:language}` (go | python)

## What to produce

Create `specs/openapi/${input:serviceName}.yml` following the style of
`specs/openapi/api.yml` (there is no separate template). The project has one read service: if
the endpoints serve the frontend, extend `specs/openapi/api.yml` instead and confirm with the
user before creating a new spec.

## Guidelines

- Define only what is described in the purpose — do not add speculative endpoints.
- Every mutating operation (`POST`, `PUT`, `PATCH`, `DELETE`) must have a request body schema.
- Reuse `components/schemas`, `components/parameters` and the shared `Error` response via `$ref`; small list wrappers may stay inline as in `api.yml`.
- All schema objects must include `required` arrays.
- Add pagination (`limit` / `offset`) to any list endpoint.
- IDs are `type: integer` (path IDs with `minimum: 1`).
- Include `/healthz` and `/readyz` paths (copy from `api.yml`).
- Add a one-line `summary` to every operation and a description to non-obvious schema properties.

## After writing the spec

List the operations defined and confirm with the user before any implementation begins.
