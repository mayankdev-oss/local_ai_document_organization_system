package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"docunest/internal/database"
	"docunest/internal/models"
	"github.com/gorilla/mux"
)

func ConfirmDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	docID, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Invalid document ID", http.StatusBadRequest)
		return
	}

	// Verify the document belongs to this user and is pending review
	var currentStatus string
	err = database.DB.QueryRow(
		"SELECT status FROM documents WHERE id = $1 AND user_id = $2",
		docID, userID,
	).Scan(&currentStatus)
	if err != nil {
		http.Error(w, "Document not found or access denied", http.StatusNotFound)
		return
	}
	if currentStatus != "needs_review" {
		http.Error(w, "Document is not pending review", http.StatusBadRequest)
		return
	}

	// Limit body size to prevent large payload attacks
	r.Body = http.MaxBytesReader(w, r.Body, 1<<15) // 32 KB max
	var req models.ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// Input validation
	if len(req.PersonName) == 0 || len(req.PersonName) > 255 {
		http.Error(w, "Person name must be between 1 and 255 characters", http.StatusBadRequest)
		return
	}
	if len(req.DocumentType) == 0 || len(req.DocumentType) > 100 {
		http.Error(w, "Document type must be between 1 and 100 characters", http.StatusBadRequest)
		return
	}
	if len(req.DOB) > 50 {
		http.Error(w, "DOB must be under 50 characters", http.StatusBadRequest)
		return
	}
	if len(req.DocumentIDNumber) > 100 {
		http.Error(w, "Document ID Number must be under 100 characters", http.StatusBadRequest)
		return
	}
	if len(req.CustomerID) > 50 {
		http.Error(w, "Invalid customer ID", http.StatusBadRequest)
		return
	}

	finalCustomerID := req.CustomerID

	if finalCustomerID == "new" || finalCustomerID == "" {
		// Create a new customer using a cryptographic UUID from the DB serial
		// The name is user-provided, not AI-provided — user has already verified it
		var newID string
		err = database.DB.QueryRow(
			"INSERT INTO customers (id, user_id, name) VALUES (gen_random_uuid()::text, $1, $2) RETURNING id",
			userID, req.PersonName,
		).Scan(&newID)
		if err != nil {
			log.Printf("Failed to create new customer: %v", err)
			http.Error(w, "Failed to create new customer", http.StatusInternalServerError)
			return
		}
		finalCustomerID = newID
	} else {
		// IDOR check: ensure the provided customer_id belongs to this user
		var exists bool
		err = database.DB.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM customers WHERE id = $1 AND user_id = $2)",
			finalCustomerID, userID,
		).Scan(&exists)
		if err != nil || !exists {
			http.Error(w, "Customer not found or access denied", http.StatusForbidden)
			return
		}
	}

	_, err = database.DB.Exec(`
		UPDATE documents 
		SET document_type = $1, person_name = $2, dob = $3, document_id_number = $4, customer_id = $5, status = 'completed'
		WHERE id = $6 AND user_id = $7
	`, req.DocumentType, req.PersonName, req.DOB, req.DocumentIDNumber, finalCustomerID, docID, userID)
	if err != nil {
		log.Printf("Failed to update document %d: %v", docID, err)
		http.Error(w, "Failed to update document", http.StatusInternalServerError)
		return
	}

	// Audit log — record the human review action
	detailsJSON, _ := json.Marshal(map[string]string{
		"person_name":        req.PersonName,
		"document_type":      req.DocumentType,
		"dob":                req.DOB,
		"document_id_number": req.DocumentIDNumber,
		"customer_id":        finalCustomerID,
	})

	_, err = database.DB.Exec(`
		INSERT INTO audit_logs (user_id, document_id, action, details)
		VALUES ($1, $2, $3, $4)
	`, userID, docID, "confirm_ai_review", string(detailsJSON))
	if err != nil {
		// Non-fatal: log the failure but don't roll back the document confirmation
		log.Printf("Warning: failed to write audit log for doc %d: %v", docID, err)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Document reviewed and completed successfully"})
}
