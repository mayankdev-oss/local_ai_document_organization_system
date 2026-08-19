package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"

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

	role, _ := r.Context().Value(RoleKey).(string)

	var rows *sql.Rows
	var err error

	if role == "admin" {
		rows, err = database.DB.Query(`
			SELECT d.id, d.filename, d.original_name, d.status, d.document_type, d.person_name, d.dob, d.document_id_number, d.confidence, d.created_at, d.ocr_text, c.name 
			FROM documents d 
			LEFT JOIN customers c ON d.customer_id = c.id 
			ORDER BY d.created_at DESC LIMIT 100
		`)
	} else {
		rows, err = database.DB.Query(`
			SELECT d.id, d.filename, d.original_name, d.status, d.document_type, d.person_name, d.dob, d.document_id_number, d.confidence, d.created_at, d.ocr_text, c.name 
			FROM documents d 
			LEFT JOIN customers c ON d.customer_id = c.id 
			WHERE d.user_id = $1
			ORDER BY d.created_at DESC LIMIT 50
		`, userID)
	}
	if err != nil {
		log.Printf("Failed to fetch documents: %v", err)
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
			&doc.DocumentType, &doc.PersonName, &doc.DOB, &doc.DocumentIDNumber, &doc.Confidence, &doc.CreatedAt, &doc.OCRText, &doc.CustomerName,
		); err != nil {
			http.Error(w, "Failed to parse document", http.StatusInternalServerError)
			return
		}
		documents = append(documents, doc)
	}

	w.Header().Set("Content-Type", "application/json")
	if documents == nil {
		documents = []DocumentWithCustomer{}
	}
	json.NewEncoder(w).Encode(documents)
}

// ViewDocument streams a document file to the browser after verifying ownership.
// Files are served with Content-Disposition: inline to allow browser viewing,
// but X-Content-Type-Options: nosniff is set via SecurityHeaders middleware
// to prevent MIME sniffing of served content.
func ViewDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	docID := vars["id"]

	// Fetch filepath and verify ownership in one query
	role, _ := r.Context().Value(RoleKey).(string)

	var storedPath string
	var err error

	if role == "admin" {
		err = database.DB.QueryRow(
			"SELECT filepath FROM documents WHERE id = $1",
			docID,
		).Scan(&storedPath)
	} else {
		err = database.DB.QueryRow(
			"SELECT filepath FROM documents WHERE id = $1 AND user_id = $2",
			docID, userID,
		).Scan(&storedPath)
	}
	if err != nil {
		http.Error(w, "Document not found or access denied", http.StatusNotFound)
		return
	}

	// Sanitize path: resolve to absolute and ensure it stays within uploads dir
	absPath, err := filepath.Abs(storedPath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	absUploads, err := filepath.Abs("./uploads")
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Reject any path that doesn't begin with the uploads directory
	if len(absPath) <= len(absUploads) || absPath[:len(absUploads)] != absUploads {
		log.Printf("Path traversal attempt detected for doc %s: resolved to %s", docID, absPath)
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	LogEvent(userID, "document_viewed", map[string]interface{}{"document_id": docID})

	// Serve inline for browser preview; X-Content-Type-Options is set by middleware
	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, absPath)
}
