---
mode: agent
description: Create a new numbered SQL migration file pair following project conventions
---

Create a new database migration for: **${input:migrationDescription}**

## Requirements

1. Find the highest existing migration number in `db/migrations/` and increment by 1.
2. Create two files:
   - `db/migrations/<NNN>_${input:migrationSlug}.up.sql`
   - `db/migrations/<NNN>_${input:migrationSlug}.down.sql`

## Conventions

**up.sql template:**
```sql
BEGIN;

-- Migration: ${input:migrationDescription}
-- Applied: run by golang-migrate

-- ... DDL statements here ...

COMMIT;
```

**down.sql template:**
```sql
BEGIN;

-- Rollback: ${input:migrationDescription}

-- ... REVERSE DDL statements here ...

COMMIT;
```

## Rules

- Wrap everything in `BEGIN; ... COMMIT;`.
- Every new table must include:
  ```sql
  id         BIGSERIAL PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  ```
- Create indexes explicitly — do not rely on implicit index creation.
- `down.sql` must exactly reverse `up.sql` (drop tables/columns/indexes created in up).
- Never use `DROP COLUMN` in `up.sql` — only `ADD COLUMN`; mark old columns with a comment `-- deprecated`.
- Use `snake_case` for all identifiers.
- Add a comment block at the top of each file describing what it does.

Generate both migration files with complete, correct SQL.
