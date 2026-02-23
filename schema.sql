CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  username VARCHAR(255) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  salt VARCHAR(255) NOT NULL
  role VARCHAR(255) NOT NULL CHECK (role IN ('admin', 'merchant', 'customer'))
)

CREATE TABLE products (
  id BIGSERIAL PRIMARY KEY,
  product_name VARCHAR(255) NOT NULL,
  seller_id INT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (seller_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_products_seller_id ON products(seller_id);

-- TODO: lock stock design
CREATE TABLE skus (
  id BIGSERIAL PRIMARY KEY,
  product_id BIGINT NOT NULL,
  attrs JSONB,
  unit_price NUMERIC(10, 2) NOT NULL CHECK (unit_price >= 0),
  stock_quantity BIGINT NOT NULL CHECK (stock_quantity >= 0),
  reserved_quantity BIGINT NOT NULL CHECK (stock_quantity >= reserved_quantity),
  FOREIGN KEY (product_id) REFERENCES products(id) ON DELETE CASCADE
);

CREATE INDEX idx_skus_product_id ON skus(product_id);