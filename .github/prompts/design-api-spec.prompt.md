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

Create `specs/openapi/${input:serviceName}.yml` following the template in
`specs/openapi/_template.yml`.

## Guidelines

- Define only what is described in the purpose — do not add speculative endpoints.
- Every mutating operation (`POST`, `PUT`, `PATCH`, `DELETE`) must have a request body schema.
- Every response must reference a `$ref` schema — no inline `type: object` in responses.
- All schema objects must include `required` arrays and `additionalProperties: false`.
- Add pagination (`limit` / `offset`) to any list endpoint.
- Use `int64` for all ID fields.
- Include `/healthz` and `/readyz` paths (copy from template).
- `operationId` values must be camelCase, unique across the file, and match what will become
  the handler/function name in the implementation.
- Add a one-line description to every path, operation, and schema property.

## After writing the spec

List the operations defined and confirm with the user before any implementation begins.
