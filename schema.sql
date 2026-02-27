-- auth service.
CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  username VARCHAR(255) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  salt VARCHAR(255) NOT NULL,
  role VARCHAR(255) NOT NULL CHECK (role IN ('admin', 'merchant', 'customer'))
)

-- product service.
CREATE TYPE products_status AS ENUM (
  'unspecified',
  'draft',
  'pending',
  'active',
  'offline'
);

CREATE TABLE products (
  id BIGSERIAL PRIMARY KEY,
  product_name VARCHAR(255) NOT NULL,
  status products_status NOT NULL DEFAULT 'draft',
  seller_id INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
);

CREATE INDEX idx_products_seller_id ON products(seller_id);

CREATE TABLE skus (
  id BIGSERIAL PRIMARY KEY,
  product_id BIGINT NOT NULL,
  attrs JSONB,
  unit_price NUMERIC(10, 2) NOT NULL CHECK (unit_price >= 0),
  FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
);

CREATE INDEX idx_skus_product_id ON skus(product_id);

-- inventory service. 
CREATE TABLE inventory (,
  sku_id BIGINT NOT NULL PRIMARY KEY,
  stock_quantity BIGINT NOT NULL CHECK (stock_quantity >= 0),
  reserved_quantity BIGINT NOT NULL CHECK (reserved_quantity >= 0),
)

CREATE TABLE inventory_lock (
  order_id BIGINT NOT NULL PRIMARY KEY,
  -- sku_id and quantity.
  payload NOT NULL JSONB
)

-- order service.
-- TODO: coupons
CREATE TABLE orders (
  id BIGSERIAL PRIMARY KEY,
  user_id INT NOT NULL,
  total NUMERIC(10, 2) NOT NULL,
  status VARCHAR(255) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'paid', 'cancelled', 'refunded')),
  payment_method VARCHAR(255) CHECK (payment_method IN ('paypal', 'stripe', 'alipay', 'wechat')),
  payment_id VARCHAR(255),
  paid_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_orders_user_id ON orders(user_id);

CREATE TABLE order_items(
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL,
  sku_id BIGINT NOT NULL,
  quantity INT NOT NULL CHECK (quantity > 0),
  -- preserve the historical name and price at time of purchase
  item_name VARCHAR(255) NOT NULL,
  unit_price NUMERIC(10, 2) NOT NULL CHECK (unit_price >= 0),
  FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE
);

CREATE INDEX idx_order_items_order_id ON order_items(order_id);