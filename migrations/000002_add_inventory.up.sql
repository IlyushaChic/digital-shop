CREATE TABLE IF NOT EXISTS inventory (
    sku         TEXT PRIMARY KEY REFERENCES products(sku) ON DELETE CASCADE,
    quantity    INTEGER NOT NULL DEFAULT 0,
    updated_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_inventory_sku ON inventory(sku);

INSERT INTO inventory (sku, quantity)
SELECT sku, 100 FROM products
ON CONFLICT (sku) DO UPDATE SET quantity = EXCLUDED.quantity;