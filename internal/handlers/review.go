package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"docunest/internal/database"
	"github.com/gorilla/mux"
)

type ReviewRequest struct {
	PersonName   string `json:"person_name"`
	DocumentType string `json:"document_type"`
	CustomerID   string `json:"customer_id"`
}

func ConfirmDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	docIDStr := vars["id"]
	docID, err := strconv.Atoi(docIDStr)
	if err != nil {
		http.Error(w, "Invalid document ID", http.StatusBadRequest)
		return
	}

	// Verify the document belongs to the user
	var currentStatus string
	err = database.DB.QueryRow("SELECT status FROM documents WHERE id = $1 AND user_id = $2", docID, userID).Scan(&currentStatus)
	if err != nil {
		http.Error(w, "Document not found or access denied", http.StatusNotFound)
		return
	}
	if currentStatus != "needs_review" {
		http.Error(w, "Document is not pending review", http.StatusBadRequest)
		return
	}

	var req ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	finalCustomerID := req.CustomerID
	
	// Create new customer if requested
	if finalCustomerID == "new" || finalCustomerID == "" {
		newID := fmt.Sprintf("cust_%d", time.Now().UnixNano())
		_, err = database.DB.Exec("INSERT INTO customers (id, user_id, name) VALUES ($1, $2, $3)", newID, userID, req.PersonName)
		if err != nil {
			http.Error(w, "Failed to create new customer", http.StatusInternalServerError)
			return
		}
		finalCustomerID = newID
	}

	// Update document
	_, err = database.DB.Exec(`
		UPDATE documents 
		SET document_type = $1, person_name = $2, customer_id = $3, status = 'completed'
		WHERE id = $4 AND user_id = $5
	`, req.DocumentType, req.PersonName, finalCustomerID, docID, userID)

	if err != nil {
		http.Error(w, "Failed to update document", http.StatusInternalServerError)
		return
	}

	// Audit Log
	detailsJSON, _ := json.Marshal(map[string]string{
		"person_name":   req.PersonName,
		"document_type": req.DocumentType,
		"customer_id":   finalCustomerID,
	})
	
	_, err = database.DB.Exec(`
		INSERT INTO audit_logs (user_id, document_id, action, details)
		VALUES ($1, $2, $3, $4)
	`, userID, docID, "confirm_ai_review", string(detailsJSON))
	
	if err != nil {
		fmt.Printf("Warning: failed to write audit log for doc %d: %v\n", docID, err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Document reviewed and completed successfully"})
}
