package services

import (
	"context"
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

func (s *DeliveryService) ProcessDelivery(orderID string) error {
	logger.Log.Infof("Processing delivery for order %s", orderID)

	tx, err := s.db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var order models.Order
	err = tx.Get(&order, "SELECT * FROM orders WHERE id = $1 FOR UPDATE", orderID)
	if err != nil {
		return err
	}

	if order.Status == models.StatusDelivered {
		return nil
	}

	if order.Status != models.StatusPaid &&
		order.Status != models.StatusRetrying &&
		order.Status != models.StatusOutOfStock &&
		order.Status != models.StatusDeliveryFailed {
		return nil
	}

	requestID := orderID

	var attemptCount int
	err = tx.Get(&attemptCount, "SELECT COUNT(*) FROM delivery_attempts WHERE order_id = $1", orderID)
	if err != nil {
		attemptCount = 0
	}

	//  проверяем лимит попыток (при order.Status ==retrying)
	if order.Status == models.StatusRetrying {
		if attemptCount >= s.maxRetries {
			_, err = tx.Exec("UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2",
				models.StatusDeliveryFailed, orderID)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`
				INSERT INTO delivery_attempts (order_id, request_id, supplier, status, error_msg)
				VALUES ($1, $2, $3, $4, $5)
			`, orderID, requestID, "both", "failed", "max retries exceeded")
			if err != nil {
				return err
			}
			return tx.Commit()
		}
	}

	// Переводим в delivering
	_, err = tx.Exec("UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2",
		models.StatusDelivering, orderID)
	if err != nil {
		return err
	}

	// Вызываем поставщика A с таймаутом 2 секунды
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req := IssueRequest{
		RequestID: requestID,
		SKU:       order.Sku,
		OrderID:   orderID,
	}

	resp, err := s.supplierA.Issue(ctx, req)

	// Обработка ошибок A
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			logger.Log.Warnf("Supplier A timeout for order %s, scheduling retry", orderID)
			_, err = tx.Exec(`
				INSERT INTO delivery_attempts (order_id, request_id, supplier, status, error_msg)
				VALUES ($1, $2, $3, $4, $5)
			`, orderID, requestID, "A", "timeout", "request timeout")
			if err != nil {
				return err
			}
			_, err = tx.Exec("UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2",
				models.StatusRetrying, orderID)
			if err != nil {
				return err
			}
			err = tx.Commit()
			if err != nil {
				return err
			}
			backoff := time.Duration(1<<uint(attemptCount)) * time.Second
			if backoff > 8*time.Second {
				backoff = 8 * time.Second
			}
			time.AfterFunc(backoff, func() {
				logger.Log.Infof("Retrying delivery for order %s after %v", orderID, backoff)
				_ = s.ProcessDelivery(orderID)
			})
			return nil
		}
		// Другая ошибка – пробуем B
		logger.Log.Warnf("Supplier A error for order %s: %v, trying B", orderID, err)
		_, err = tx.Exec(`
			INSERT INTO delivery_attempts (order_id, request_id, supplier, status, error_msg)
			VALUES ($1, $2, $3, $4, $5)
		`, orderID, requestID, "A", "error", err.Error())
		if err != nil {
			return err
		}

		// Вызываем B
		resp, err = s.supplierB.Issue(ctx, req)
		if err != nil {
			logger.Log.Errorf("Supplier B also failed for order %s: %v", orderID, err)
			_, err = tx.Exec(`
				INSERT INTO delivery_attempts (order_id, request_id, supplier, status, error_msg)
				VALUES ($1, $2, $3, $4, $5)
			`, orderID, requestID, "B", "error", err.Error())
			if err != nil {
				return err
			}
			_, err = tx.Exec("UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2",
				models.StatusDeliveryFailed, orderID)
			if err != nil {
				return err
			}
			return tx.Commit()
		}
		if resp.Status != "ok" {
			_, err = tx.Exec(`
				INSERT INTO delivery_attempts (order_id, request_id, supplier, status, error_msg)
				VALUES ($1, $2, $3, $4, $5)
			`, orderID, requestID, "B", "error", resp.Reason)
			if err != nil {
				return err
			}
			_, err = tx.Exec("UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2",
				models.StatusDeliveryFailed, orderID)
			if err != nil {
				return err
			}
			return tx.Commit()
		}
		// B success
		code := resp.Code
		_, err = tx.Exec(`
			UPDATE orders SET status = $1, delivered_code = $2, updated_at = NOW() WHERE id = $3
		`, models.StatusDelivered, code, orderID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
			INSERT INTO delivery_attempts (order_id, request_id, supplier, status, code)
			VALUES ($1, $2, $3, $4, $5)
		`, orderID, requestID, "B", "success", code)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
			INSERT INTO money_journal (order_id, event_type, amount, currency)
			VALUES ($1, $2, $3, $4)
		`, orderID, "delivery_success", order.Amount, order.Currency)
		if err != nil {
			return err
		}
		return tx.Commit()
	}

	// Успех от A
	if resp.Status != "ok" {
		logger.Log.Warnf("Supplier A returned error: %s, trying B", resp.Reason)
		_, err = tx.Exec(`
			INSERT INTO delivery_attempts (order_id, request_id, supplier, status, error_msg)
			VALUES ($1, $2, $3, $4, $5)
		`, orderID, requestID, "A", "error", resp.Reason)
		if err != nil {
			return err
		}
		// B
		resp, err = s.supplierB.Issue(ctx, req)
		if err != nil || resp.Status != "ok" {
			if err != nil {
				_, err = tx.Exec(`
					INSERT INTO delivery_attempts (order_id, request_id, supplier, status, error_msg)
					VALUES ($1, $2, $3, $4, $5)
				`, orderID, requestID, "B", "error", err.Error())
			} else {
				_, err = tx.Exec(`
					INSERT INTO delivery_attempts (order_id, request_id, supplier, status, error_msg)
					VALUES ($1, $2, $3, $4, $5)
				`, orderID, requestID, "B", "error", resp.Reason)
			}
			if err != nil {
				return err
			}
			_, err = tx.Exec("UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2",
				models.StatusDeliveryFailed, orderID)
			if err != nil {
				return err
			}
			return tx.Commit()
		}
		// B success
		code := resp.Code
		_, err = tx.Exec(`
			UPDATE orders SET status = $1, delivered_code = $2, updated_at = NOW() WHERE id = $3
		`, models.StatusDelivered, code, orderID)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
			INSERT INTO delivery_attempts (order_id, request_id, supplier, status, code)
			VALUES ($1, $2, $3, $4, $5)
		`, orderID, requestID, "B", "success", code)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`
			INSERT INTO money_journal (order_id, event_type, amount, currency)
			VALUES ($1, $2, $3, $4)
		`, orderID, "delivery_success", order.Amount, order.Currency)
		if err != nil {
			return err
		}
		return tx.Commit()
	}

	// Успех от A
	code := resp.Code
	_, err = tx.Exec(`
		UPDATE orders SET status = $1, delivered_code = $2, updated_at = NOW() WHERE id = $3
	`, models.StatusDelivered, code, orderID)
	if err != nil {
		return err
	}
	logger.Log.Infof("A success, inserting attempt for order %s, code %s", orderID, code)
	_, err = tx.Exec(`
		INSERT INTO delivery_attempts (order_id, request_id, supplier, status, code)
		VALUES ($1, $2, $3, $4, $5)
	`, orderID, requestID, "A", "success", code)
	if err != nil {
		logger.Log.Errorf("Failed to insert delivery_attempts: %v", err)
		return err
	}
	logger.Log.Infof("Attempt inserted successfully")
	_, err = tx.Exec(`
		INSERT INTO money_journal (order_id, event_type, amount, currency)
		VALUES ($1, $2, $3, $4)
	`, orderID, "delivery_success", order.Amount, order.Currency)
	if err != nil {
		logger.Log.Errorf("Failed to insert money_journal: %v", err)
		return err
	}
	return tx.Commit()
}
