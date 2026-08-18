package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"docunest/internal/database"
	"docunest/internal/models"
	"github.com/gorilla/mux"
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

func GetCustomerDocuments(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	customerID := vars["id"]

	// Verify customer belongs to user
	var exists bool
	err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM customers WHERE id = $1 AND user_id = $2)", customerID, userID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Customer not found or access denied", http.StatusNotFound)
		return
	}

	rows, err := database.DB.Query(`
		SELECT id, original_name, status, document_type, person_name, created_at 
		FROM documents 
		WHERE customer_id = $1 AND user_id = $2
		ORDER BY created_at DESC
	`, customerID, userID)
	
	if err != nil {
		http.Error(w, "Failed to fetch documents", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var doc models.Document
		if err := rows.Scan(&doc.ID, &doc.OriginalName, &doc.Status, &doc.DocumentType, &doc.PersonName, &doc.CreatedAt); err != nil {
			http.Error(w, "Failed to scan document", http.StatusInternalServerError)
			return
		}
		documents = append(documents, doc)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(documents)
}
