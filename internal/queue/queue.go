package queue

import (
	"context"
	"shop/internal/logger"
	"shop/internal/services"
	"sync"
)

func StartWorkerPool(ctx context.Context, wg *sync.WaitGroup, queue <-chan int, deliverySvc *services.DeliveryService, workers int) {
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case itemID := <-queue:
					if err := deliverySvc.ProcessItem(itemID); err != nil {
						logger.Log.Errorf("Failed to process item %d: %v", itemID, err)
					}
				}
			}
		}()
	}
}
