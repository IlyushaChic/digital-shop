package services

import (
	"database/sql"
	"fmt"
	"shop/internal/models"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type OrderService struct {
	db *sqlx.DB
}

func NewOrderService(db *sqlx.DB) *OrderService {
	return &OrderService{db: db}
}

type CreateOrderRequest struct {
	Items []ItemRequest `json:"items"`
}

type ItemRequest struct {
	Sku      string `json:"sku"`
	Quantity int    `json:"quantity"`
}

func (s *OrderService) CreateOrder(req CreateOrderRequest) (*models.Order, []models.OrderItem, error) {
	if len(req.Items) == 0 {
		return nil, nil, fmt.Errorf("at least one item required")
	}

	priceMap := make(map[string]models.Product)
	for _, it := range req.Items {
		var product models.Product
		err := s.db.Get(&product, "SELECT sku, price, currency FROM products WHERE sku = $1", it.Sku)
		if err != nil {
			return nil, nil, fmt.Errorf("product %s not found: %w", it.Sku, err)
		}
		priceMap[it.Sku] = product
	}

	totalAmount := 0
	for _, it := range req.Items {
		product := priceMap[it.Sku]
		totalAmount += product.Price * it.Quantity
	}

	orderID := "ord_" + uuid.New().String()[:8]
	now := time.Now()

	order := &models.Order{
		ID:        orderID,
		Sku:       "",
		Status:    models.StatusCreated,
		Amount:    totalAmount,
		Currency:  "RUB",
		CreatedAt: now,
		UpdatedAt: now,
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()

	_, err = tx.NamedExec(`
		INSERT INTO orders (id, sku, status, amount, currency, created_at, updated_at)
		VALUES (:id, :sku, :status, :amount, :currency, :created_at, :updated_at)
	`, order)
	if err != nil {
		return nil, nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO money_journal (order_id, event_type, amount, currency)
		VALUES ($1, $2, $3, $4)
	`, orderID, "order_created", totalAmount, order.Currency)
	if err != nil {
		return nil, nil, err
	}

	orderItems := make([]models.OrderItem, 0, len(req.Items))
	for _, it := range req.Items {
		product := priceMap[it.Sku]
		supplier := "supplier_a"
		if it.Sku == "STEAM-TOPUP-500" {
			supplier = "supplier_b"
		}

		var itemID int
		err := tx.QueryRow(`
			INSERT INTO order_items (order_id, sku, supplier, quantity, price, currency, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id
		`, orderID, it.Sku, supplier, it.Quantity, product.Price, product.Currency, models.ItemStatusPending, now, now).Scan(&itemID)
		if err != nil {
			return nil, nil, err
		}

		item := models.OrderItem{
			ID:        itemID,
			OrderID:   orderID,
			Sku:       it.Sku,
			Supplier:  supplier,
			Quantity:  it.Quantity,
			Price:     product.Price,
			Currency:  product.Currency,
			Status:    models.ItemStatusPending,
			CreatedAt: now,
			UpdatedAt: now,
		}
		orderItems = append(orderItems, item)
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}

	return order, orderItems, nil
}

func (s *OrderService) GetOrder(id string) (*models.Order, error) {
	var order models.Order
	err := s.db.Get(&order, "SELECT * FROM orders WHERE id = $1", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &order, err
}

func (s *OrderService) GetOrderWithItems(id string) (*models.Order, []models.OrderItem, error) {
	order, err := s.GetOrder(id)
	if err != nil || order == nil {
		return order, nil, err
	}
	items, err := s.GetOrderItems(id)
	return order, items, err
}

func (s *OrderService) GetOrderItems(orderID string) ([]models.OrderItem, error) {
	var items []models.OrderItem
	err := s.db.Select(&items, `
		SELECT * FROM order_items WHERE order_id = $1 ORDER BY id
	`, orderID)
	if err != nil {
		return nil, err
	}
	return items, nil
}

func (s *OrderService) UpdateOrderStatus(orderID string) error {
	items, err := s.GetOrderItems(orderID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	var deliveredCount, refundedCount int
	for _, it := range items {
		switch it.Status {
		case models.ItemStatusDelivered:
			deliveredCount++
		case models.ItemStatusRefunded:
			refundedCount++
		}
	}

	total := len(items)
	var newStatus string
	if deliveredCount == total {
		newStatus = models.StatusDelivered
	} else if refundedCount == total {
		newStatus = models.StatusFullyRefunded
	} else if deliveredCount > 0 && refundedCount > 0 {
		newStatus = models.StatusPartiallyDelivered
	} else {
		newStatus = models.StatusPaid
	}

	_, err = s.db.Exec(`
		UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2
	`, newStatus, orderID)
	return err
}

func (s *OrderService) GetPendingItems(orderID string) ([]models.OrderItem, error) {
	var items []models.OrderItem
	err := s.db.Select(&items, `
		SELECT * FROM order_items WHERE order_id = $1 AND status = 'pending'
	`, orderID)
	return items, err
}
