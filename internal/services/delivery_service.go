package services

import (
	"context"
	"database/sql"
	"fmt"
	"shop/internal/logger"
	"shop/internal/models"
	"time"

	"github.com/jmoiron/sqlx"
)

type DeliveryService struct {
	db         *sqlx.DB
	supplierA  Supplier
	supplierB  Supplier
	maxRetries int
}

func NewDeliveryService(db *sqlx.DB, supplierA, supplierB Supplier) *DeliveryService {
	return &DeliveryService{
		db:         db,
		supplierA:  supplierA,
		supplierB:  supplierB,
		maxRetries: 3,
	}
}

func (s *DeliveryService) ProcessItem(itemID int) error {
	logger.Log.Infof("Processing order item %d", itemID)

	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var item models.OrderItem
	err = tx.Get(&item, "SELECT * FROM order_items WHERE id = $1 FOR UPDATE", itemID)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("order item %d not found", itemID)
		}
		return err
	}

	if item.Status == models.ItemStatusDelivered || item.Status == models.ItemStatusRefunded {
		return nil
	}
	if item.Status != models.ItemStatusPending && item.Status != models.ItemStatusFailed {
		return nil
	}

	supplier := s.supplierA
	if item.Sku == "STEAM-TOPUP-500" {
		supplier = s.supplierB
	}

	requestID := fmt.Sprintf("req_%d", itemID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := IssueRequest{
		RequestID: requestID,
		SKU:       item.Sku,
		OrderID:   item.OrderID,
	}

	resp, err := supplier.Issue(ctx, req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Log.Warnf("Supplier timeout for item %d, scheduling retry", itemID)
			time.AfterFunc(1*time.Second, func() {
				_ = s.ProcessItem(itemID)
			})
			return nil
		}
		logger.Log.Warnf("Supplier error for item %d: %v, refunding", itemID, err)
		return s.refundItem(tx, item, "delivery_failed")
	}

	if resp.Status != "ok" || resp.Code == "" {
		logger.Log.Warnf("Supplier returned error for item %d: %s", itemID, resp.Reason)
		return s.refundItem(tx, item, "delivery_failed")
	}

	code := resp.Code
	now := time.Now()
	_, err = tx.Exec(`
		UPDATE order_items 
		SET status = $1, code = $2, request_id = $3, delivered_at = $4, updated_at = NOW()
		WHERE id = $5
	`, models.ItemStatusDelivered, code, requestID, now, itemID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO money_journal (order_id, event_type, amount, currency)
		VALUES ($1, $2, $3, $4)
	`, item.OrderID, "item_delivered", item.Price*item.Quantity, item.Currency)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	orderSvc := NewOrderService(s.db)
	if err := orderSvc.UpdateOrderStatus(item.OrderID); err != nil {
		logger.Log.Errorf("Failed to update order status for %s: %v", item.OrderID, err)
	}

	logger.Log.Infof("Item %d delivered with code %s", itemID, code)
	return nil
}

func (s *DeliveryService) refundItem(tx *sqlx.Tx, item models.OrderItem, reason string) error {
	_, err := tx.Exec(`
		UPDATE order_items SET status = $1, refunded_at = NOW(), updated_at = NOW()
		WHERE id = $2
	`, models.ItemStatusRefunded, item.ID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO refunds (order_id, order_item_id, amount, currency, reason)
		VALUES ($1, $2, $3, $4, $5)
	`, item.OrderID, item.ID, item.Price*item.Quantity, item.Currency, reason)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO money_journal (order_id, event_type, amount, currency)
		VALUES ($1, $2, $3, $4)
	`, item.OrderID, "item_refunded", item.Price*item.Quantity, item.Currency)
	if err != nil {
		return err
	}

	return nil
}
