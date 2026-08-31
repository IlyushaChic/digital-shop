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

func (s *OrderService) CreateOrder(sku string) (*models.Order, error) {
	var product models.Product
	err := s.db.Get(&product, "SELECT sku, price, currency FROM products WHERE sku = $1", sku)
	if err != nil {
		return nil, fmt.Errorf("product not found: %w", err)
	}

	orderID := "ord_" + uuid.New().String()[:8]
	now := time.Now()
	order := &models.Order{
		ID:        orderID,
		Sku:       sku,
		Status:    models.StatusCreated,
		Amount:    product.Price,
		Currency:  product.Currency,
		CreatedAt: now,
		UpdatedAt: now,
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.NamedExec(`
		INSERT INTO orders (id, sku, status, amount, currency, created_at, updated_at)
		VALUES (:id, :sku, :status, :amount, :currency, :created_at, :updated_at)
	`, order)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		INSERT INTO money_journal (order_id, event_type, amount, currency)
		VALUES ($1, $2, $3, $4)
	`, orderID, "order_created", product.Price, product.Currency)
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, err
	}

	return order, nil
}

func (s *OrderService) GetOrder(id string) (*models.Order, error) {
	var order models.Order
	err := s.db.Get(&order, "SELECT * FROM orders WHERE id = $1", id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &order, err
}
