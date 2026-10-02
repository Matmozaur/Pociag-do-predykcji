-- 012_train_operations_updated_at.down.sql
-- Reverse 012_train_operations_updated_at.up.sql.

BEGIN;

DROP INDEX IF EXISTS idx_train_operations_updated_at;

COMMIT;
