package queue

import (
	"context"
	"shop/internal/logger"
	"shop/internal/services"
	"sync"
)

func StartWorkerPool(ctx context.Context, wg *sync.WaitGroup, queue <-chan string, deliverySvc *services.DeliveryService, workers int) {
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case orderID := <-queue:
					if err := deliverySvc.ProcessDelivery(orderID); err != nil {
						logger.Log.Errorf("Delivery failed for order %s: %v", orderID, err)
					}
				}
			}
		}()
	}
}
