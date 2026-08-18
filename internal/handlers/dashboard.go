package handlers

import (
	"encoding/json"
	"net/http"

	"docunest/internal/database"
)

func GetStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var stats struct {
		TotalDocuments int `json:"total_documents"`
		TotalCustomers int `json:"total_customers"`
		ProcessedToday int `json:"processed_today"`
	}

	err := database.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE user_id = $1", userID).Scan(&stats.TotalDocuments)
	if err != nil {
		http.Error(w, "Failed to get total documents", http.StatusInternalServerError)
		return
	}

	err = database.DB.QueryRow("SELECT COUNT(*) FROM customers WHERE user_id = $1", userID).Scan(&stats.TotalCustomers)
	if err != nil {
		http.Error(w, "Failed to get total customers", http.StatusInternalServerError)
		return
	}

	err = database.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE user_id = $1 AND DATE(created_at) = CURRENT_DATE", userID).Scan(&stats.ProcessedToday)
	if err != nil {
		http.Error(w, "Failed to get today's documents", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}
