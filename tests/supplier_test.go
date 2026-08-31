package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"shop/internal/config"
	"shop/internal/db"
	"shop/internal/handlers"
	"shop/internal/logger"
	"shop/internal/models"
	"shop/internal/queue"
	"shop/internal/services"
	"shop/internal/supplier_mock"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeoutAndRetry(t *testing.T) {
	logger.Init()
	cfg := config.Load()
	cfg.DBPort = 5433

	database, err := db.Connect(cfg)
	require.NoError(t, err)
	defer database.Close()

	setupTestDB(t, database)

	mockA := supplier_mock.NewMockSupplier()
	mockA.Delay = 3 * time.Second

	mockB := supplier_mock.NewMockSupplier()
	mockB.Delay = 0

	serverA := httptest.NewServer(http.HandlerFunc(mockA.Handler))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(mockB.Handler))
	defer serverB.Close()

	supplierA := services.NewHTTPClient(serverA.URL, 2*time.Second)
	supplierB := services.NewHTTPClient(serverB.URL, 2*time.Second)

	deliverySvc := services.NewDeliveryService(database, supplierA, supplierB)

	taskQueue := make(chan string, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	queue.StartWorkerPool(ctx, &wg, taskQueue, deliverySvc, 1)

	orderSvc := services.NewOrderService(database)
	webhookHandler := handlers.NewWebhookHandler(database, deliverySvc, taskQueue)
	orderHandler := handlers.NewOrderHandler(orderSvc)

	r := chi.NewRouter()
	r.Post("/api/orders", orderHandler.CreateOrder)
	r.Post("/webhook/payment", webhookHandler.HandlePayment)

	ts := httptest.NewServer(r)
	defer ts.Close()

	createReq := map[string]string{"sku": "KEY-CS2-PRIME"}
	body, _ := json.Marshal(createReq)
	resp, err := http.Post(ts.URL+"/api/orders", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var orderResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&orderResp)
	require.NoError(t, err)
	orderID := orderResp["ID"].(string)
	t.Logf("Created order: %s", orderID)

	payload := map[string]interface{}{
		"event_id":   "evt_test",
		"order_id":   orderID,
		"status":     "paid",
		"amount":     1290,
		"currency":   "RUB",
		"created_at": "2025-01-01T12:00:00Z",
	}
	b, _ := json.Marshal(payload)
	resp, err = http.Post(ts.URL+"/webhook/payment", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(6 * time.Second)

	var order models.Order
	err = database.Get(&order, "SELECT * FROM orders WHERE id = $1", orderID)
	require.NoError(t, err)
	t.Logf("Order status: %s", order.Status)
	assert.Equal(t, models.StatusDelivered, order.Status)
	assert.NotNil(t, order.DeliveredCode)

	var successAttempts int
	err = database.Get(&successAttempts,
		"SELECT COUNT(*) FROM delivery_attempts WHERE order_id = $1 AND status = 'success'",
		orderID)
	require.NoError(t, err)
	t.Logf("successAttempts = %d", successAttempts)
	assert.Equal(t, 1, successAttempts)

	var moneyCount int
	err = database.Get(&moneyCount,
		"SELECT COUNT(*) FROM money_journal WHERE order_id = $1 AND event_type = 'delivery_success'",
		orderID)
	require.NoError(t, err)
	assert.Equal(t, 1, moneyCount)
}

func TestFallbackAB(t *testing.T) {
	logger.Init()
	cfg := config.Load()
	cfg.DBPort = 5433

	database, err := db.Connect(cfg)
	require.NoError(t, err)
	defer database.Close()

	setupTestDB(t, database)

	mockA := supplier_mock.NewMockSupplier()
	mockA.AlwaysError = true
	mockB := supplier_mock.NewMockSupplier()
	mockB.Delay = 0

	serverA := httptest.NewServer(http.HandlerFunc(mockA.Handler))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(mockB.Handler))
	defer serverB.Close()

	supplierA := services.NewHTTPClient(serverA.URL, 2*time.Second)
	supplierB := services.NewHTTPClient(serverB.URL, 2*time.Second)

	deliverySvc := services.NewDeliveryService(database, supplierA, supplierB)

	taskQueue := make(chan string, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	queue.StartWorkerPool(ctx, &wg, taskQueue, deliverySvc, 1)

	orderSvc := services.NewOrderService(database)
	webhookHandler := handlers.NewWebhookHandler(database, deliverySvc, taskQueue)
	orderHandler := handlers.NewOrderHandler(orderSvc)

	r := chi.NewRouter()
	r.Post("/api/orders", orderHandler.CreateOrder)
	r.Post("/webhook/payment", webhookHandler.HandlePayment)

	ts := httptest.NewServer(r)
	defer ts.Close()

	createReq := map[string]string{"sku": "KEY-CS2-PRIME"}
	body, _ := json.Marshal(createReq)
	resp, err := http.Post(ts.URL+"/api/orders", "application/json", bytes.NewReader(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	var orderResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&orderResp)
	require.NoError(t, err)
	orderID := orderResp["ID"].(string)

	payload := map[string]interface{}{
		"event_id":   "evt_fallback",
		"order_id":   orderID,
		"status":     "paid",
		"amount":     1290,
		"currency":   "RUB",
		"created_at": "2025-01-01T12:00:00Z",
	}
	b, _ := json.Marshal(payload)
	resp, err = http.Post(ts.URL+"/webhook/payment", "application/json", bytes.NewReader(b))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	time.Sleep(2 * time.Second)

	var order models.Order
	err = database.Get(&order, "SELECT * FROM orders WHERE id = $1", orderID)
	require.NoError(t, err)
	assert.Equal(t, models.StatusDelivered, order.Status)
	assert.NotNil(t, order.DeliveredCode)

	var attempts []struct {
		Supplier string `db:"supplier"`
		Status   string `db:"status"`
	}
	err = database.Select(&attempts, "SELECT supplier, status FROM delivery_attempts WHERE order_id = $1 ORDER BY id", orderID)
	require.NoError(t, err)
	assert.Len(t, attempts, 2)
	assert.Equal(t, "A", attempts[0].Supplier)
	assert.Equal(t, "error", attempts[0].Status)
	assert.Equal(t, "B", attempts[1].Supplier)
	assert.Equal(t, "success", attempts[1].Status)
}
