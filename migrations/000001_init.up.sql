CREATE TABLE IF NOT EXISTS products (
    sku         TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    type        TEXT NOT NULL,
    price       INTEGER NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'RUB',
    image       TEXT
);

CREATE TABLE IF NOT EXISTS orders (
    id            TEXT PRIMARY KEY,
    sku           TEXT NOT NULL REFERENCES products(sku),
    status        TEXT NOT NULL DEFAULT 'created',
    amount        INTEGER NOT NULL,
    currency      TEXT NOT NULL DEFAULT 'RUB',
    delivered_code TEXT,
    created_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at    TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Таблица ключей (пул)
CREATE TABLE IF NOT EXISTS keys (
    id          SERIAL PRIMARY KEY,
    code        TEXT UNIQUE NOT NULL,
    used        BOOLEAN NOT NULL DEFAULT FALSE,
    order_id    TEXT REFERENCES orders(id) ON DELETE SET NULL,
    assigned_at TIMESTAMP WITH TIME ZONE
);

CREATE TABLE IF NOT EXISTS webhook_events (
    event_id    TEXT PRIMARY KEY,
    order_id    TEXT NOT NULL REFERENCES orders(id),
    status      TEXT NOT NULL,
    processed   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS delivery_attempts (
    id          SERIAL PRIMARY KEY,
    order_id    TEXT NOT NULL REFERENCES orders(id),
    request_id  TEXT NOT NULL,
    supplier    TEXT NOT NULL,
    status      TEXT NOT NULL, -- 'pending', 'success', 'failed', 'timeout'
    code        TEXT,
    error_msg   TEXT,
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS money_journal (
    id          SERIAL PRIMARY KEY,
    order_id    TEXT NOT NULL REFERENCES orders(id),
    event_type  TEXT NOT NULL, -- 'payment_received', 'delivery_success', 'payment_failed'
    amount      INTEGER NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'RUB',
    created_at  TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_orders_status ON orders(status);
CREATE INDEX idx_orders_sku ON orders(sku);
CREATE INDEX idx_webhook_order_id ON webhook_events(order_id);
CREATE INDEX idx_keys_used ON keys(used);