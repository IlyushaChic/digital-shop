CREATE TABLE IF NOT EXISTS order_items (
    id            SERIAL PRIMARY KEY,
    order_id      TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    sku           TEXT NOT NULL REFERENCES products(sku),
    supplier      TEXT NOT NULL,
    quantity      INTEGER NOT NULL DEFAULT 1,
    price         INTEGER NOT NULL,
    status        TEXT NOT NULL DEFAULT 'pending',
    code          TEXT,
    request_id    TEXT,
    delivered_at  TIMESTAMP,
    refunded_at   TIMESTAMP,
    created_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_order_items_order_id ON order_items(order_id);
CREATE INDEX idx_order_items_status ON order_items(status);
CREATE INDEX idx_order_items_request_id ON order_items(request_id);

CREATE TABLE IF NOT EXISTS refunds (
    id            SERIAL PRIMARY KEY,
    order_id      TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    order_item_id INTEGER REFERENCES order_items(id) ON DELETE SET NULL,
    amount        INTEGER NOT NULL,
    currency      TEXT NOT NULL DEFAULT 'RUB',
    reason        TEXT NOT NULL,
    created_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_refunds_order_id ON refunds(order_id);