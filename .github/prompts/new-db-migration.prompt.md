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
- Follow `001_init.up.sql`: tables keyed by the PLK id when PLK has a stable one (`stations`),
  otherwise `id BIGSERIAL PRIMARY KEY` plus a `UNIQUE` natural key (`trains`, `operations`);
  child rows use a composite key (`(train_id, seq)`). Add `updated_at TIMESTAMPTZ NOT NULL
  DEFAULT now()` where syncs touch the row.
- Create indexes explicitly for new foreign keys and the predicates both repositories filter
  on (`airflow/plugins/pociag_processing/repository.py`, `services/go/api/internal/repository`).
- `down.sql` must exactly reverse `up.sql` (drop tables/columns/indexes created in up).
- Never use `DROP COLUMN` in `up.sql` — only `ADD COLUMN`; mark old columns with a comment `-- deprecated`.
- Use `snake_case` for all identifiers.
- Add a comment block at the top of each file describing what it does.

Generate both migration files with complete, correct SQL, update both repositories if the
shape they read or write changed, and run both SQL test suites (`db/migrations/CLAUDE.md`).
