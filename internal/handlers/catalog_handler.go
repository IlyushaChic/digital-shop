package handlers

import (
	"encoding/json"
	"net/http"
	"shop/internal/services"
)

type CatalogHandler struct {
	svc *services.CatalogService
}

func NewCatalogHandler(svc *services.CatalogService) *CatalogHandler {
	return &CatalogHandler{svc: svc}
}

func (h *CatalogHandler) GetCatalog(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.GetCatalog()
	if err != nil {
		http.Error(w, "failed to get catalog", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}
