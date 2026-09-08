package handlers

import (
	"encoding/json"
	"net/http"
	"shop/internal/models"
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
	var req services.CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Items) == 0 {
		http.Error(w, "at least one item required", http.StatusBadRequest)
		return
	}

	order, items, err := h.orderSvc.CreateOrder(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := struct {
		Order *models.Order      `json:"order"`
		Items []models.OrderItem `json:"items"`
	}{
		Order: order,
		Items: items,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
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
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(order)
}

func (h *OrderHandler) GetOrderWithItems(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	order, items, err := h.orderSvc.GetOrderWithItems(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if order == nil {
		http.NotFound(w, r)
		return
	}
	response := struct {
		Order *models.Order      `json:"order"`
		Items []models.OrderItem `json:"items"`
	}{
		Order: order,
		Items: items,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
