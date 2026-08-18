package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"docunest/internal/database"
	"docunest/internal/models"
)

func GetCustomers(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	query := r.URL.Query().Get("q")
	
	var rows *sql.Rows
	var err error
	
	if query != "" {
		rows, err = database.DB.Query("SELECT id, name, created_at FROM customers WHERE user_id = $1 AND name ILIKE $2 ORDER BY name ASC LIMIT 20", userID, "%"+query+"%")
	} else {
		rows, err = database.DB.Query("SELECT id, name, created_at FROM customers WHERE user_id = $1 ORDER BY name ASC LIMIT 20", userID)
	}

	if err != nil {
		http.Error(w, "Failed to query customers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var customers []models.Customer
	for rows.Next() {
		var c models.Customer
		if err := rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
			http.Error(w, "Failed to scan customer", http.StatusInternalServerError)
			return
		}
		customers = append(customers, c)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(customers)
}
