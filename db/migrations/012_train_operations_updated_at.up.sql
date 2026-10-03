-- 012_train_operations_updated_at.up.sql
-- Active trains (data-service listActiveOperations) filter on
-- updated_at >= MAX(train_operations.updated_at) - 15 minutes. Without this index the MAX is a
-- sequential scan of the whole table; with it, a single backward index probe.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_train_operations_updated_at
    ON train_operations (updated_at);

COMMIT;
