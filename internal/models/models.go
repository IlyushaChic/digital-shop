package models

import "time"

const (
	StatusCreated            = "created"
	StatusPaid               = "paid"
	StatusDelivering         = "delivering"
	StatusRetrying           = "retrying"
	StatusDelivered          = "delivered"
	StatusPaymentFailed      = "payment_failed"
	StatusOutOfStock         = "out_of_stock"
	StatusDeliveryFailed     = "delivery_failed"
	StatusPartiallyDelivered = "partially_delivered"
	StatusFullyRefunded      = "fully_refunded"
)

const (
	ItemStatusPending   = "pending"
	ItemStatusDelivered = "delivered"
	ItemStatusRefunded  = "refunded"
	ItemStatusFailed    = "failed"
)

type Product struct {
	Sku      string `db:"sku"`
	Name     string `db:"name"`
	Type     string `db:"type"`
	Price    int    `db:"price"`
	Currency string `db:"currency"`
	Image    string `db:"image"`
}

type Order struct {
	ID            string    `db:"id"`
	Sku           string    `db:"sku"` // deprecated, теперь в позициях
	Status        string    `db:"status"`
	Amount        int       `db:"amount"`
	Currency      string    `db:"currency"`
	DeliveredCode *string   `db:"delivered_code"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type OrderItem struct {
	ID          int        `db:"id"`
	OrderID     string     `db:"order_id"`
	Sku         string     `db:"sku"`
	Supplier    string     `db:"supplier"`
	Quantity    int        `db:"quantity"`
	Price       int        `db:"price"`
	Currency    string     `db:"currency"`
	Status      string     `db:"status"`
	Code        *string    `db:"code"`
	RequestID   *string    `db:"request_id"`
	DeliveredAt *time.Time `db:"delivered_at"`
	RefundedAt  *time.Time `db:"refunded_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

type Key struct {
	ID         int        `db:"id"`
	Code       string     `db:"code"`
	Used       bool       `db:"used"`
	OrderID    *string    `db:"order_id"`
	AssignedAt *time.Time `db:"assigned_at"`
}

type WebhookEvent struct {
	EventID   string    `db:"event_id"`
	OrderID   string    `db:"order_id"`
	Status    string    `db:"status"`
	Processed bool      `db:"processed"`
	CreatedAt time.Time `db:"created_at"`
}

type DeliveryAttempt struct {
	ID        int       `db:"id"`
	OrderID   string    `db:"order_id"`
	RequestID string    `db:"request_id"`
	Supplier  string    `db:"supplier"`
	Status    string    `db:"status"`
	Code      *string   `db:"code"`
	ErrorMsg  *string   `db:"error_msg"`
	CreatedAt time.Time `db:"created_at"`
}

type MoneyJournal struct {
	ID        int       `db:"id"`
	OrderID   string    `db:"order_id"`
	EventType string    `db:"event_type"`
	Amount    int       `db:"amount"`
	Currency  string    `db:"currency"`
	CreatedAt time.Time `db:"created_at"`
}

type Refund struct {
	ID          int       `db:"id"`
	OrderID     string    `db:"order_id"`
	OrderItemID *int      `db:"order_item_id"`
	Amount      int       `db:"amount"`
	Currency    string    `db:"currency"`
	Reason      string    `db:"reason"`
	CreatedAt   time.Time `db:"created_at"`
}
