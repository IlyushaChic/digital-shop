package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T, db *sqlx.DB) {
	_, err := db.Exec("TRUNCATE TABLE money_journal, delivery_attempts, webhook_events, orders CASCADE")
	require.NoError(t, err)

	_, err = db.Exec("DELETE FROM keys")
	require.NoError(t, err)

	_, err = db.Exec(`
		INSERT INTO keys (code) VALUES 
		('LFXC-TNCS-BPCD'), ('P3EI-W8UO-9B4K'), ('FEL3-GUXN-TCCH'), ('YPLV-QK2Z-IUS5'),
		('0K9E-P1FR-BY1U'), ('5LZV-UQ48-RXCZ'), ('X93K-NYAQ-GEC1'), ('EIO5-CQT5-35KO'),
		('M58F-GIIR-VJAP'), ('NU8Y-SWYB-6252'), ('OODW-CCHF-MBAF'), ('DNA5-WFJM-NE49'),
		('QRDD-MJ3F-A8TF'), ('TAT9-5ZJN-G1T2'), ('LI39-4330-ISMB'), ('BKJY-8Q79-8NHI'),
		('HHW6-4RX2-DX62'), ('1RG2-L28O-O80G'), ('EF63-F39X-MTEA'), ('8XS7-P53H-JKIV'),
		('JPE6-MQV6-P7ST'), ('SAPG-A2GR-0ULS'), ('T2DU-IJ1S-U16P'), ('WSSY-QTR7-Z57J'),
		('U74E-EPCI-CY26'), ('FZXF-58H8-OR93'), ('FPSM-HLZA-TPAL'), ('WSC9-28DJ-B2JE'),
		('P63J-F7UZ-DCYP'), ('C7W2-D4C5-QMT7'), ('JESI-DFBH-LK1K'), ('SGMA-JA0T-GR7D'),
		('3PR4-OSY9-M3ZW'), ('OMBE-C0JF-D45Y'), ('KIKQ-FQJ8-9TI8'), ('LMAN-RSHS-AJDO'),
		('BAKI-VT1X-Z5OL'), ('9F0X-B46W-03FS'), ('S423-V6YY-IBEM'), ('D4UW-WYRA-20ST'),
		('XC0J-CJ0H-09RN'), ('RY1W-XCFJ-0KUA'), ('CJYY-YKSQ-QE6H'), ('97AQ-38QJ-H8HU'),
		('FS8E-3S5Z-I6RA'), ('ARQK-FML4-A14E'), ('7Z6K-NO9V-MPJB'), ('D4K7-IJSG-N853'),
		('W67T-ZB0Q-1XKB'), ('7EQM-K09J-XKUO')
	`)
	require.NoError(t, err)
}

type simpleSupplier struct{}

func (s simpleSupplier) Issue(ctx context.Context, req services.IssueRequest) (services.IssueResponse, error) {
	return services.IssueResponse{Status: "ok", Code: "SIMPLE-CODE"}, nil
}

func TestRaceCondition(t *testing.T) {
	logger.Init()

	cfg := config.Load()
	cfg.DBPort = 5433

	database, err := db.Connect(cfg)
	require.NoError(t, err)
	defer database.Close()

	setupTestDB(t, database)

	var freeKeys int
	err = database.Get(&freeKeys, "SELECT COUNT(*) FROM keys WHERE used = false")
	require.NoError(t, err)
	t.Logf("Свободных ключей после подготовки: %d", freeKeys)

	supplierA := simpleSupplier{}
	supplierB := simpleSupplier{}

	orderSvc := services.NewOrderService(database)
	deliverySvc := services.NewDeliveryService(database, supplierA, supplierB)

	taskQueue := make(chan string, 100)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	queue.StartWorkerPool(ctx, &wg, taskQueue, deliverySvc, 5)

	orderHandler := handlers.NewOrderHandler(orderSvc)
	webhookHandler := handlers.NewWebhookHandler(database, deliverySvc, taskQueue)

	r := chi.NewRouter()
	r.Post("/api/orders", orderHandler.CreateOrder)
	r.Get("/api/orders/{id}", orderHandler.GetOrder)
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

	var wgRequests sync.WaitGroup
	const numRequests = 50
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < numRequests; i++ {
		wgRequests.Add(1)
		go func(i int) {
			defer wgRequests.Done()
			eventID := fmt.Sprintf("evt_%d", i)
			payload := map[string]interface{}{
				"event_id":   eventID,
				"order_id":   orderID,
				"status":     "paid",
				"amount":     1290,
				"currency":   "RUB",
				"created_at": "2025-01-01T12:00:00Z",
			}
			b, _ := json.Marshal(payload)
			resp, err := http.Post(ts.URL+"/webhook/payment", "application/json", bytes.NewReader(b))
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					mu.Lock()
					successCount++
					mu.Unlock()
				}
			}
		}(i)
	}
	wgRequests.Wait()

	time.Sleep(10 * time.Second)

	var order models.Order
	err = database.Get(&order, "SELECT * FROM orders WHERE id = $1", orderID)
	require.NoError(t, err)
	t.Logf("Статус заказа: %s", order.Status)
	assert.Equal(t, models.StatusDelivered, order.Status, "статус должен быть delivered")
	assert.NotNil(t, order.DeliveredCode, "должен быть выдан код")

	var successAttempts int
	err = database.Get(&successAttempts,
		"SELECT COUNT(*) FROM delivery_attempts WHERE order_id = $1 AND status = 'success'",
		orderID)
	require.NoError(t, err)
	assert.Equal(t, 1, successAttempts, "ровно одна успешная попытка выдачи")

	var moneyCount int
	err = database.Get(&moneyCount,
		"SELECT COUNT(*) FROM money_journal WHERE order_id = $1 AND event_type = 'delivery_success'",
		orderID)
	require.NoError(t, err)
	assert.Equal(t, 1, moneyCount, "должна быть ровно одна запись успешной выдачи")

	t.Logf("Всего успешных ответов от вебхуков: %d из %d", successCount, numRequests)
}

func TestIdempotentWebhookSameEvent(t *testing.T) {
	logger.Init()
	cfg := config.Load()
	cfg.DBPort = 5433

	database, err := db.Connect(cfg)
	require.NoError(t, err)
	defer database.Close()

	setupTestDB(t, database)

	supplierA := simpleSupplier{}
	supplierB := simpleSupplier{}

	orderSvc := services.NewOrderService(database)
	deliverySvc := services.NewDeliveryService(database, supplierA, supplierB)
	taskQueue := make(chan string, 100)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	queue.StartWorkerPool(ctx, &wg, taskQueue, deliverySvc, 5)

	orderHandler := handlers.NewOrderHandler(orderSvc)
	webhookHandler := handlers.NewWebhookHandler(database, deliverySvc, taskQueue)

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
	json.NewDecoder(resp.Body).Decode(&orderResp)
	orderID := orderResp["ID"].(string)

	const numRequests = 50
	var wgRequests sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		wgRequests.Add(1)
		go func() {
			defer wgRequests.Done()
			payload := map[string]interface{}{
				"event_id":   "evt_same",
				"order_id":   orderID,
				"status":     "paid",
				"amount":     1290,
				"currency":   "RUB",
				"created_at": "2025-01-01T12:00:00Z",
			}
			b, _ := json.Marshal(payload)
			resp, err := http.Post(ts.URL+"/webhook/payment", "application/json", bytes.NewReader(b))
			if err == nil {
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Errorf("ожидался 200, получил %d", resp.StatusCode)
				}
			}
		}()
	}
	wgRequests.Wait()

	time.Sleep(10 * time.Second)

	var order models.Order
	err = database.Get(&order, "SELECT * FROM orders WHERE id = $1", orderID)
	require.NoError(t, err)
	assert.Equal(t, models.StatusDelivered, order.Status)
	assert.NotNil(t, order.DeliveredCode)

	var successAttempts int
	database.Get(&successAttempts,
		"SELECT COUNT(*) FROM delivery_attempts WHERE order_id = $1 AND status = 'success'",
		orderID)
	assert.Equal(t, 1, successAttempts)

	var moneyCount int
	database.Get(&moneyCount,
		"SELECT COUNT(*) FROM money_journal WHERE order_id = $1 AND event_type = 'delivery_success'",
		orderID)
	assert.Equal(t, 1, moneyCount)
}
