CREATE INDEX idx_orders_status_updated ON orders(status, updated_at);
CREATE INDEX idx_money_journal_event_type ON money_journal(event_type);