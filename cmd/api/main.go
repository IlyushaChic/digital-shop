package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"time"

	"shop/internal/config"
	"shop/internal/db"
	"shop/internal/handlers"
	"shop/internal/logger"
	"shop/internal/queue"
	"shop/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	logger.Init()
	log := logger.Log

	cfg := config.Load()
	database, err := db.Connect(cfg)
	if err != nil {
		log.Fatalf("DB connect: %v", err)
	}
	defer database.Close()

	// Поставщики (заглушки)
	supplierA := services.NewHTTPClient("http://localhost:8081", 2*time.Second)
	supplierB := services.NewHTTPClient("http://localhost:8082", 2*time.Second)

	orderSvc := services.NewOrderService(database)
	deliverySvc := services.NewDeliveryService(database, supplierA, supplierB)
	reconciliationSvc := services.NewReconciliationService(database)

	taskQueue := make(chan int, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// Воркеры для позиций
	queue.StartWorkerPool(ctx, &wg, taskQueue, deliverySvc, 5)

	// Временно отключаем recovery worker, чтобы не править reconciliation и recovery
	// recoveryWorker := services.NewRecoveryWorker(reconciliationSvc, deliverySvc, 30*time.Second)
	// recoveryWorker.Start(ctx)

	// Хендлеры
	orderHandler := handlers.NewOrderHandler(orderSvc)
	webhookHandler := handlers.NewWebhookHandler(database, deliverySvc, taskQueue)
	reconciliationHandler := handlers.NewReconciliationHandler(reconciliationSvc)

	r := chi.NewRouter()
	r.Post("/api/orders", orderHandler.CreateOrder)
	r.Get("/api/orders/{id}", orderHandler.GetOrder)
	r.Get("/api/orders/{id}/items", orderHandler.GetOrderWithItems)
	r.Post("/webhook/payment", webhookHandler.HandlePayment)
	r.Get("/api/reconciliation", reconciliationHandler.GetReport)

	srv := &http.Server{
		Addr:    ":" + cfg.HTTPPort,
		Handler: r,
	}

	go func() {
		log.Infof("Server starting on :%s", cfg.HTTPPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Info("Shutting down server...")
	cancel()
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := srv.Shutdown(ctxShutdown); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}
	wg.Wait()
	log.Info("Server exited")
}
