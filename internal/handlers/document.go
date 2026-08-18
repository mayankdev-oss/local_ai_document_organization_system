package handlers

import (
	"encoding/json"
	"net/http"

	"docunest/internal/database"
	"docunest/internal/models"
	"github.com/gorilla/mux"
)

func GetDocuments(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := database.DB.Query(`
		SELECT d.id, d.filename, d.original_name, d.status, d.document_type, d.person_name, d.confidence, d.created_at, c.name 
		FROM documents d 
		LEFT JOIN customers c ON d.customer_id = c.id 
		WHERE d.user_id = $1
		ORDER BY d.created_at DESC LIMIT 50
	`, userID)
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

func ViewDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	docID := vars["id"]

	var filepath string
	err := database.DB.QueryRow("SELECT filepath FROM documents WHERE id = $1 AND user_id = $2", docID, userID).Scan(&filepath)
	if err != nil {
		http.Error(w, "Document not found or access denied", http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, filepath)
}
