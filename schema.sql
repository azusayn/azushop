-- auth service.
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  username VARCHAR(255) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  salt VARCHAR(255) NOT NULL,
  role VARCHAR(255) NOT NULL CHECK (role IN ('admin', 'merchant', 'customer'))
)

-- product service.
CREATE TYPE product_status AS ENUM (
  'draft',
  'pending',
  'active',
  'offline'
);

CREATE TABLE products (
  id UUID PRIMARY KEY NOT NULL,
  product_name VARCHAR(255) NOT NULL,
  status product_status NOT NULL,
  seller_id INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
);

CREATE INDEX idx_products_seller_id ON products(seller_id);

CREATE TABLE skus (
  -- UUIDv7
  id UUID PRIMARY KEY NOT NULL, 
  product_id UUID NOT NULL,
  attrs JSONB,
  unit_price NUMERIC(10, 2) NOT NULL CHECK (unit_price >= 0),
  FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
);

CREATE INDEX idx_skus_product_id ON skus(product_id);

-- inventory service. 
CREATE TABLE inventory (,
  sku_id UUID NOT NULL PRIMARY KEY,
  stock_quantity BIGINT NOT NULL CHECK (stock_quantity >= 0),
  reserved_quantity BIGINT NOT NULL CHECK (reserved_quantity >= 0),
)

CREATE TYPE inventory_lock_status AS ENUM (
  'locked',
  'confirmed',
  'released'
)

CREATE TABLE inventory_lock (
  order_id BIGINT NOT NULL PRIMARY KEY,
  -- mapping from sku_id to quantity.
  payload NOT NULL JSONB,
  status inventory_lock_status NOT NULL,
)

-- order service.
CREATE TYPE order_status AS ENUM (
  'pending',
  'cancelled'
  'confirmed',
  'completed',
)

-- TODO(3): coupons & currency
-- currently use cny as default currency.
-- currency ref: https://docs.stripe.com/currencies#zero-decimal.
CREATE TABLE orders (
  id BIGSERIAL PRIMARY KEY,
  user_id INT NOT NULL,
  -- without tax, TODO(3): this should be renamed to 'subtotal'.
  total NUMERIC(10, 2) NOT NULL,
  status order_status NOT NULL,
  order_items JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- payment service.
CREATE TYPE payment_method AS ENUM (
  'stripe',
  'alipay',
  'wechat'
)

CREATE TYPE payment_status AS ENUM (
  'pending',
  'cancelled',
  'paid',
  'refunding',
  'refunded'
)

CREATE TABLE payments (
  id BIGSERIAL NOT NULL PRIMARY KEY,
  -- id from payment provider.
  external_id text NOT NULL,
  order_id BIGINT NOT NULL,
  user_id INT NOT NULL,
  method payment_method NOT NULL,
  status payment_status NOT NULL,
  amount_total DECIMAL(10, 2) NOT NULL CHECK (amount_total >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
)

CREATE INDEX idx_payments_user_id ON payments(user_id);
CREATE UNIQUE INDEX idx_payments_external_id ON payments(external_id);
CREATE INDEX idx_payments_order_id ON payments(order_id);
