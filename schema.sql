CREATE TABLE users (
  id SERIAL PRIMARY KEY,
  username VARCHAR(255) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  salt VARCHAR(255) NOT NULL
)

CREATE TABLE products(
  id BIGSERIAL PRIMARY KEY,
  product_name VARCHAR(255) NOT NULL,
  seller_id INT NOT NULL,
  stock_quantity INT NOT NULL CHECK (stock_quantity >= 0),
  unit_price NUMERIC(10, 2) NOT NULL CHECK (unit_price >= 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  FOREIGN KEY (seller_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX idx_products_seller_id ON products(seller_id);