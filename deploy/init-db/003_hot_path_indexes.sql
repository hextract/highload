CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_restaurants_name_active_trgm
ON restaurants
USING gin (name gin_trgm_ops)
WHERE is_active = true;

CREATE INDEX IF NOT EXISTS idx_payments_order ON payments (order_id);
