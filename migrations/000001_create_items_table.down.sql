DROP TRIGGER IF EXISTS set_items_timestamp ON items;
DROP FUNCTION IF EXISTS trigger_set_timestamp();
DROP INDEX IF EXISTS idx_items_sku;
DROP TABLE IF EXISTS items;
-- DROP EXTENSION IF EXISTS "uuid-ossp"; -- Only if no other table uses it
