package services

import (
	"shop/internal/models"

	"github.com/jmoiron/sqlx"
)

type ReconciliationService struct {
	db *sqlx.DB
}

func NewReconciliationService(db *sqlx.DB) *ReconciliationService {
	return &ReconciliationService{db: db}
}

type ReconciliationReport struct {
	NotDelivered            []models.Order `json:"not_delivered"`
	DeliveredWithoutPayment []models.Order `json:"delivered_without_payment"`
	TotalPaymentReceived    int64          `json:"total_payment_received"`
	TotalDeliverySuccess    int64          `json:"total_delivery_success"`
	Difference              int64          `json:"difference"`
}

func (s *ReconciliationService) GetReport() (*ReconciliationReport, error) {
	var notDelivered []models.Order
	err := s.db.Select(&notDelivered, `
		SELECT * FROM orders 
		WHERE status IN ($1, $2, $3, $4)
		ORDER BY created_at DESC
	`, models.StatusPaid, models.StatusDelivering, models.StatusOutOfStock, models.StatusDeliveryFailed)
	if err != nil {
		return nil, err
	}

	var deliveredWithoutPayment []models.Order
	err = s.db.Select(&deliveredWithoutPayment, `
		SELECT o.* FROM orders o
		LEFT JOIN money_journal m ON o.id = m.order_id AND m.event_type = 'payment_received'
		WHERE o.status = $1 AND m.id IS NULL
	`, models.StatusDelivered)
	if err != nil {
		return nil, err
	}

	var totalPayment int64
	err = s.db.Get(&totalPayment, `
		SELECT COALESCE(SUM(amount), 0) FROM money_journal WHERE event_type = 'payment_received'
	`)
	if err != nil {
		return nil, err
	}

	var totalDelivery int64
	err = s.db.Get(&totalDelivery, `
		SELECT COALESCE(SUM(amount), 0) FROM money_journal WHERE event_type = 'delivery_success'
	`)
	if err != nil {
		return nil, err
	}

	return &ReconciliationReport{
		NotDelivered:            notDelivered,
		DeliveredWithoutPayment: deliveredWithoutPayment,
		TotalPaymentReceived:    totalPayment,
		TotalDeliverySuccess:    totalDelivery,
		Difference:              totalPayment - totalDelivery,
	}, nil
}
