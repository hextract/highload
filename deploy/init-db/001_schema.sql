CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE restaurants (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    cuisine_type VARCHAR(50) NOT NULL,
    address TEXT,
    lat DECIMAL(9,6) NOT NULL,
    lon DECIMAL(9,6) NOT NULL,
    rating DECIMAL(2,1) NOT NULL DEFAULT 4.0,
    avg_price INT NOT NULL DEFAULT 500,
    is_active BOOLEAN NOT NULL DEFAULT true,
    working_hours JSONB DEFAULT '{}',
    image_url TEXT,
    delivery_time_min INT NOT NULL DEFAULT 40,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE menu_categories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    restaurant_id UUID NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE TABLE menu_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category_id UUID NOT NULL REFERENCES menu_categories (id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    price INT NOT NULL,
    image_url TEXT,
    is_available BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE orders (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID,
    restaurant_id UUID NOT NULL REFERENCES restaurants (id),
    courier_id UUID,
    status VARCHAR(30) NOT NULL DEFAULT 'created',
    delivery_address TEXT NOT NULL,
    delivery_lat DECIMAL(9,6) NOT NULL,
    delivery_lon DECIMAL(9,6) NOT NULL,
    total_amount DECIMAL(10,2) NOT NULL,
    comment TEXT,
    estimated_delivery_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE order_items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id UUID NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    menu_item_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    quantity INT NOT NULL,
    unit_price DECIMAL(10,2) NOT NULL,
    total_price DECIMAL(10,2) NOT NULL
);

CREATE TABLE payments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    order_id UUID NOT NULL REFERENCES orders (id),
    amount DECIMAL(10,2) NOT NULL,
    currency CHAR(3) NOT NULL DEFAULT 'RUB',
    status VARCHAR(30) NOT NULL DEFAULT 'pending',
    payment_method VARCHAR(20),
    provider VARCHAR(50) DEFAULT 'mock',
    provider_tx_id VARCHAR(255),
    idempotency_key UUID NOT NULL,
    error_code VARCHAR(50),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (idempotency_key)
);

CREATE TABLE saga_state (
    order_id UUID PRIMARY KEY REFERENCES orders (id) ON DELETE CASCADE,
    current_step VARCHAR(50) NOT NULL DEFAULT 'created',
    status VARCHAR(20) NOT NULL DEFAULT 'running',
    step_data JSONB DEFAULT '{}',
    retry_count INT NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE EXTENSION IF NOT EXISTS cube;
CREATE EXTENSION IF NOT EXISTS earthdistance;

CREATE INDEX idx_restaurants_geo ON restaurants USING gist (
    ll_to_earth(lat::double precision, lon::double precision)
);

CREATE INDEX idx_restaurants_cuisine_rating ON restaurants (cuisine_type, rating DESC)
    WHERE is_active = true;

CREATE INDEX idx_orders_user_created ON orders (user_id, created_at DESC);

CREATE INDEX idx_menu_categories_restaurant ON menu_categories (restaurant_id);
CREATE INDEX idx_menu_items_category ON menu_items (category_id);
CREATE INDEX idx_order_items_order ON order_items (order_id);
