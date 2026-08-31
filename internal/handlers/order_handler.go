package handlers

import (
	"encoding/json"
	"net/http"
	"shop/internal/services"

	"github.com/go-chi/chi/v5"
)

type OrderHandler struct {
	orderSvc *services.OrderService
}

func NewOrderHandler(orderSvc *services.OrderService) *OrderHandler {
	return &OrderHandler{orderSvc: orderSvc}
}

func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sku string `json:"sku"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	order, err := h.orderSvc.CreateOrder(req.Sku)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	json.NewEncoder(w).Encode(order)
}

func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	order, err := h.orderSvc.GetOrder(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if order == nil {
		http.NotFound(w, r)
		return
	}
	json.NewEncoder(w).Encode(order)
}
