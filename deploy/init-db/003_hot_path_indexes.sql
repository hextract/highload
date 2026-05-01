CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_restaurants_name_active_trgm
ON restaurants
USING gin (name gin_trgm_ops)
WHERE is_active = true;

CREATE INDEX IF NOT EXISTS idx_restaurants_geo_active ON restaurants USING gist (
    ll_to_earth(lat::double precision, lon::double precision)
) WHERE is_active = true;

CREATE INDEX IF NOT EXISTS idx_payments_order ON payments (order_id);
