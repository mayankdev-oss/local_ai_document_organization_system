package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"docunest/internal/database"
	"docunest/internal/storage"
)

func UploadDocument(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(10 << 20) // 10 MB limit
	if err != nil {
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	file, handler, err := r.FormFile("document")
	if err != nil {
		http.Error(w, "Error retrieving the file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file type
	ext := strings.ToLower(filepath.Ext(handler.Filename))
	if ext != ".pdf" && ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
		http.Error(w, "Invalid file type. Only PDF, JPG, JPEG, and PNG are allowed.", http.StatusBadRequest)
		return
	}

	// Customer logic
	customerType := r.FormValue("customer_type") // "new" or "existing"
	customerID := r.FormValue("customer_id")
	// For "new" customer, we don't have an ID yet. 
	// The plan says: "New Customer requires only the upload... Extract name and create a unique customer ID when no match exists (Phase 5)"
	// For now, in Phase 2, we just save the document.

	if customerType == "existing" && customerID == "" {
		http.Error(w, "Customer ID is required for existing customer", http.StatusBadRequest)
		return
	}

	// Save original file
	newFilename, filePath, err := storage.SaveFile(file, handler.Filename)
	if err != nil {
		http.Error(w, "Failed to save file", http.StatusInternalServerError)
		return
	}

	// Insert into DB
	var docID int
	var cID *string
	if customerType == "existing" {
		cID = &customerID
	}

	query := `INSERT INTO documents (filename, filepath, original_name, status, customer_id) 
	          VALUES ($1, $2, $3, $4, $5) RETURNING id`
	err = database.DB.QueryRow(query, newFilename, filePath, handler.Filename, "uploaded", cID).Scan(&docID)
	
	if err != nil {
		http.Error(w, "Failed to create database record", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"message": "File uploaded successfully",
		"id":      docID,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
