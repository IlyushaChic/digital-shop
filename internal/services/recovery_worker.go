package services

import (
	"context"
	"shop/internal/logger"
	"time"
)

type RecoveryWorker struct {
	svc         *ReconciliationService
	deliverySvc *DeliveryService
	interval    time.Duration
}

func NewRecoveryWorker(svc *ReconciliationService, deliverySvc *DeliveryService, interval time.Duration) *RecoveryWorker {
	return &RecoveryWorker{
		svc:         svc,
		deliverySvc: deliverySvc,
		interval:    interval,
	}
}

func (w *RecoveryWorker) Start(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				recovered, err := w.svc.RecoverPendingOrders(w.deliverySvc)
				if err != nil {
					logger.Log.Errorf("Recovery worker error: %v", err)
				} else if recovered > 0 {
					logger.Log.Infof("Recovery worker: %d orders recovered", recovered)
				}
			}
		}
	}()
}
