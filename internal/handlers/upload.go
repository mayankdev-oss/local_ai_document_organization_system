package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
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

	// Insert into DB with status "uploaded"
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

	// Trigger OCR process in a goroutine
	go processOCR(docID, filePath)

	response := map[string]interface{}{
		"message": "File uploaded successfully",
		"id":      docID,
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func processOCR(docID int, filePath string) {
	// 1. Update status to "reading"
	_, err := database.DB.Exec("UPDATE documents SET status = 'reading' WHERE id = $1", docID)
	if err != nil {
		fmt.Printf("Failed to update status to reading for doc %d: %v\n", docID, err)
		return
	}

	// 2. Call local Python OCR service
	// We need to send a multipart/form-data request with the file
	file, err := os.Open(filePath)
	if err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to open file: %v", err))
		return
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to create form file: %v", err))
		return
	}
	
	_, err = io.Copy(part, file)
	if err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to copy file: %v", err))
		return
	}
	writer.Close()

	req, err := http.NewRequest("POST", "http://127.0.0.1:8000/api/ocr", body)
	if err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to create OCR request: %v", err))
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to reach OCR service: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		updateStatusAndError(docID, "failed", fmt.Sprintf("OCR service error: %s", string(respBody)))
		return
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to decode OCR response: %v", err))
		return
	}

	// 3. Save OCR text and update status
	_, err = database.DB.Exec("UPDATE documents SET ocr_text = $1, status = 'identifying' WHERE id = $2", result.Text, docID)
	if err != nil {
		fmt.Printf("Failed to save OCR text for doc %d: %v\n", docID, err)
		return
	}
	
	fmt.Printf("Successfully completed OCR for document %d\n", docID)
	// (Phase 4 will pick up from 'identifying' status)
}

func updateStatusAndError(docID int, status, errMsg string) {
	fmt.Printf("Document %d error: %s\n", docID, errMsg)
	database.DB.Exec("UPDATE documents SET status = $1 WHERE id = $2", status, docID)
}
