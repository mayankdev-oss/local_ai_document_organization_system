package handlers

import (
	"encoding/json"
	"net/http"

	"docunest/internal/database"
	"docunest/internal/models"
)

func GetDocuments(w http.ResponseWriter, r *http.Request) {
	rows, err := database.DB.Query(`
		SELECT d.id, d.filename, d.original_name, d.status, d.document_type, d.person_name, d.confidence, d.created_at, c.name 
		FROM documents d 
		LEFT JOIN customers c ON d.customer_id = c.id 
		ORDER BY d.created_at DESC LIMIT 50
	`)
	if err != nil {
		http.Error(w, "Failed to fetch documents", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type DocumentWithCustomer struct {
		models.Document
		CustomerName *string `json:"customer_name,omitempty"`
	}

	var documents []DocumentWithCustomer
	for rows.Next() {
		var doc DocumentWithCustomer
		if err := rows.Scan(
			&doc.ID, &doc.Filename, &doc.OriginalName, &doc.Status, 
			&doc.DocumentType, &doc.PersonName, &doc.Confidence, &doc.CreatedAt, &doc.CustomerName,
		); err != nil {
			http.Error(w, "Failed to parse document", http.StatusInternalServerError)
			return
		}
		documents = append(documents, doc)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(documents)
}
