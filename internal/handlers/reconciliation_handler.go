package handlers

import (
	"encoding/json"
	"net/http"
	"shop/internal/services"
)

type ReconciliationHandler struct {
	svc *services.ReconciliationService
}

func NewReconciliationHandler(svc *services.ReconciliationService) *ReconciliationHandler {
	return &ReconciliationHandler{svc: svc}
}

func (h *ReconciliationHandler) GetReport(w http.ResponseWriter, r *http.Request) {
	report, err := h.svc.GetReport()
	if err != nil {
		http.Error(w, "failed to get report", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}
