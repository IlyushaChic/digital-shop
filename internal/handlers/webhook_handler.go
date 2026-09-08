package handlers

import (
	"encoding/json"
	"net/http"

	"shop/internal/models"
	"shop/internal/services"

	"github.com/jmoiron/sqlx"
)

type WebhookHandler struct {
	db          *sqlx.DB
	deliverySvc *services.DeliveryService
	queue       chan int // теперь канал для itemID
}

func NewWebhookHandler(db *sqlx.DB, deliverySvc *services.DeliveryService, queue chan int) *WebhookHandler {
	return &WebhookHandler{
		db:          db,
		deliverySvc: deliverySvc,
		queue:       queue,
	}
}

func (h *WebhookHandler) HandlePayment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		EventID   string `json:"event_id"`
		OrderID   string `json:"order_id"`
		Status    string `json:"status"`
		Amount    int    `json:"amount"`
		Currency  string `json:"currency"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	var processed bool
	err := h.db.Get(&processed, "SELECT processed FROM webhook_events WHERE event_id = $1", req.EventID)
	if err == nil && processed {
		w.WriteHeader(http.StatusOK)
		return
	}

	tx, err := h.db.Beginx()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	var existing models.WebhookEvent
	err = tx.Get(&existing, "SELECT * FROM webhook_events WHERE event_id = $1 FOR UPDATE", req.EventID)
	if err == nil && existing.Processed {
		tx.Commit()
		w.WriteHeader(http.StatusOK)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO webhook_events (event_id, order_id, status, created_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (event_id) DO NOTHING
	`, req.EventID, req.OrderID, req.Status)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if req.Status == "paid" {
		var order models.Order
		err = tx.Get(&order, "SELECT * FROM orders WHERE id = $1 FOR UPDATE", req.OrderID)
		if err != nil {
			tx.Commit()
			http.Error(w, "order not found", http.StatusNotFound)
			return
		}

		if order.Status == models.StatusPaid || order.Status == models.StatusDelivered {
			_, _ = tx.Exec("UPDATE webhook_events SET processed = true WHERE event_id = $1", req.EventID)
			tx.Commit()
			w.WriteHeader(http.StatusOK)
			return
		}

		_, err = tx.Exec(`
			UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2
		`, models.StatusPaid, req.OrderID)
		if err != nil {
			tx.Commit()
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		_, err = tx.Exec(`
			INSERT INTO money_journal (order_id, event_type, amount, currency)
			VALUES ($1, $2, $3, $4)
		`, req.OrderID, "payment_received", req.Amount, req.Currency)
		if err != nil {
			tx.Commit()
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		_, err = tx.Exec("UPDATE webhook_events SET processed = true WHERE event_id = $1", req.EventID)
		if err != nil {
			tx.Commit()
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		if err := tx.Commit(); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		// Получаем все pending позиции и ставим их в очередь
		var items []models.OrderItem
		err = h.db.Select(&items, "SELECT id FROM order_items WHERE order_id = $1 AND status = 'pending'", req.OrderID)
		if err == nil {
			for _, it := range items {
				h.queue <- it.ID
			}
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	if req.Status == "failed" {
		_, err = tx.Exec(`
			UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2
		`, models.StatusPaymentFailed, req.OrderID)
		if err != nil {
			tx.Commit()
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		_, err = tx.Exec(`
			INSERT INTO money_journal (order_id, event_type, amount, currency)
			VALUES ($1, $2, $3, $4)
		`, req.OrderID, "payment_failed", req.Amount, req.Currency)
		if err != nil {
			tx.Commit()
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		_, err = tx.Exec("UPDATE webhook_events SET processed = true WHERE event_id = $1", req.EventID)
		if err != nil {
			tx.Commit()
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		tx.Commit()
		w.WriteHeader(http.StatusOK)
		return
	}

	tx.Commit()
	http.Error(w, "unknown status", http.StatusBadRequest)
}
