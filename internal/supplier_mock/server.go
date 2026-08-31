package supplier_mock

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"sync"
	"time"
)

type MockSupplier struct {
	mu                      sync.Mutex
	codes                   map[string]string
	Delay                   time.Duration
	GenerateCodeBeforeDelay bool
	ErrorRate               float64
	TimeoutRate             float64
	AlwaysError             bool
	AlwaysTimeout           bool
}

func NewMockSupplier() *MockSupplier {
	return &MockSupplier{
		codes:                   make(map[string]string),
		Delay:                   0,
		GenerateCodeBeforeDelay: false,
		ErrorRate:               0,
		TimeoutRate:             0,
		AlwaysError:             false,
		AlwaysTimeout:           false,
	}
}

func (m *MockSupplier) Handler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/issue" {
		http.NotFound(w, r)
		return
	}

	if m.AlwaysTimeout || (m.TimeoutRate > 0 && rand.Float64() < m.TimeoutRate) {
		select {}
		return
	}

	var req struct {
		RequestID string `json:"request_id"`
		SKU       string `json:"sku"`
		OrderID   string `json:"order_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if code, ok := m.codes[req.RequestID]; ok {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok", "code": code})
		return
	}

	if m.AlwaysError || (m.ErrorRate > 0 && rand.Float64() < m.ErrorRate) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "reason": "internal_error"})
		return
	}

	code := fmt.Sprintf("MOCK-%s", req.RequestID[:8])
	m.codes[req.RequestID] = code

	if m.Delay > 0 {
		time.Sleep(m.Delay)
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "code": code})
}
