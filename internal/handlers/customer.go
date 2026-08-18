package handlers

import (
	"encoding/json"
	"log"
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

	var customers []models.Customer
	var err error

	if query != "" {
		// Limit search query length to prevent oversized queries
		if len(query) > 100 {
			query = query[:100]
		}
		rows, qErr := database.DB.Query(
			"SELECT id, name, created_at FROM customers WHERE user_id = $1 AND name ILIKE $2 ORDER BY name ASC LIMIT 20",
			userID, "%"+query+"%",
		)
		if qErr != nil {
			log.Printf("Failed to query customers: %v", qErr)
			http.Error(w, "Failed to query customers", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var c models.Customer
			if err = rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
				http.Error(w, "Failed to scan customer", http.StatusInternalServerError)
				return
			}
			customers = append(customers, c)
		}
	} else {
		rows, qErr := database.DB.Query(
			"SELECT id, name, created_at FROM customers WHERE user_id = $1 ORDER BY name ASC LIMIT 50",
			userID,
		)
		if qErr != nil {
			log.Printf("Failed to query customers: %v", qErr)
			http.Error(w, "Failed to query customers", http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var c models.Customer
			if err = rows.Scan(&c.ID, &c.Name, &c.CreatedAt); err != nil {
				http.Error(w, "Failed to scan customer", http.StatusInternalServerError)
				return
			}
			customers = append(customers, c)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if customers == nil {
		customers = []models.Customer{}
	}
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

	// Validate customer ID length
	if len(customerID) == 0 || len(customerID) > 50 {
		http.Error(w, "Invalid customer ID", http.StatusBadRequest)
		return
	}

	// Verify customer belongs to the authenticated user (IDOR protection)
	var exists bool
	err := database.DB.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM customers WHERE id = $1 AND user_id = $2)",
		customerID, userID,
	).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Customer not found or access denied", http.StatusNotFound)
		return
	}

	rows, err := database.DB.Query(`
		SELECT id, original_name, status, document_type, person_name, dob, document_id_number, created_at 
		FROM documents 
		WHERE customer_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT 100
	`, customerID, userID)
	if err != nil {
		log.Printf("Failed to fetch customer documents: %v", err)
		http.Error(w, "Failed to fetch documents", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var documents []models.Document
	for rows.Next() {
		var doc models.Document
		if err := rows.Scan(&doc.ID, &doc.OriginalName, &doc.Status, &doc.DocumentType, &doc.PersonName, &doc.DOB, &doc.DocumentIDNumber, &doc.CreatedAt); err != nil {
			http.Error(w, "Failed to scan document", http.StatusInternalServerError)
			return
		}
		documents = append(documents, doc)
	}

	w.Header().Set("Content-Type", "application/json")
	if documents == nil {
		documents = []models.Document{}
	}
	json.NewEncoder(w).Encode(documents)
}
