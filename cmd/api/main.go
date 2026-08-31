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
	"shop/internal/supplier_mock"

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

	mockA := supplier_mock.NewMockSupplier()
	mockA.Delay = 0
	mockA.ErrorRate = 0.0
	mockA.TimeoutRate = 0.0

	mockB := supplier_mock.NewMockSupplier()
	mockB.Delay = 0
	mockB.ErrorRate = 0.0
	mockB.TimeoutRate = 0.0

	go func() {
		muxA := http.NewServeMux()
		muxA.HandleFunc("/issue", mockA.Handler)
		log.Infof("Mock supplier A listening on :8081")
		if err := http.ListenAndServe(":8081", muxA); err != nil {
			log.Errorf("Mock supplier A error: %v", err)
		}
	}()
	go func() {
		muxB := http.NewServeMux()
		muxB.HandleFunc("/issue", mockB.Handler)
		log.Infof("Mock supplier B listening on :8082")
		if err := http.ListenAndServe(":8082", muxB); err != nil {
			log.Errorf("Mock supplier B error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	supplierA := services.NewHTTPClient("http://localhost:8081", 2*time.Second)
	supplierB := services.NewHTTPClient("http://localhost:8082", 2*time.Second)

	// Сервисы
	orderSvc := services.NewOrderService(database)
	deliverySvc := services.NewDeliveryService(database, supplierA, supplierB)
	reconciliationSvc := services.NewReconciliationService(database)
	catalogSvc := services.NewCatalogService(database)
	catalogHandler := handlers.NewCatalogHandler(catalogSvc)

	// Очередь задач
	taskQueue := make(chan string, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// Воркеры для выдачи
	queue.StartWorkerPool(ctx, &wg, taskQueue, deliverySvc, 5)

	// Recovery worker (фоновая задача по восстановлению)
	recoveryWorker := services.NewRecoveryWorker(reconciliationSvc, deliverySvc, 30*time.Second)
	recoveryWorker.Start(ctx)

	// Хендлеры
	orderHandler := handlers.NewOrderHandler(orderSvc)
	webhookHandler := handlers.NewWebhookHandler(database, deliverySvc, taskQueue)
	reconciliationHandler := handlers.NewReconciliationHandler(reconciliationSvc)

	// Роутер
	r := chi.NewRouter()
	r.Post("/api/orders", orderHandler.CreateOrder)
	r.Get("/api/orders/{id}", orderHandler.GetOrder)
	r.Post("/webhook/payment", webhookHandler.HandlePayment)
	r.Get("/api/reconciliation", reconciliationHandler.GetReport)
	r.Get("/api/catalog", catalogHandler.GetCatalog) // <-- новый эндпоинт
	// Сервер
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

	// Graceful shutdown
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
