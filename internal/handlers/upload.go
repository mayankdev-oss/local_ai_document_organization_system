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
	"sync"
	"time"

	"docunest/internal/database"
	"docunest/internal/storage"
)

var uploadRateLimiter = make(map[string]time.Time)
var limiterMutex sync.Mutex

func UploadDocument(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	clientIP := r.RemoteAddr
	limiterMutex.Lock()
	lastUpload, exists := uploadRateLimiter[clientIP]
	if exists && time.Since(lastUpload) < 5*time.Second {
		limiterMutex.Unlock()
		http.Error(w, "Rate limit exceeded. Please wait 5 seconds.", http.StatusTooManyRequests)
		return
	}
	uploadRateLimiter[clientIP] = time.Now()
	limiterMutex.Unlock()

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

	// MIME Validation
	buffer := make([]byte, 512)
	if _, err := file.Read(buffer); err != nil {
		http.Error(w, "Failed to read file", http.StatusBadRequest)
		return
	}
	file.Seek(0, 0)
	contentType := http.DetectContentType(buffer)
	if contentType != "application/pdf" && contentType != "image/jpeg" && contentType != "image/png" {
		http.Error(w, "Invalid MIME type. Only PDF, JPEG, and PNG are allowed.", http.StatusBadRequest)
		return
	}

	// Customer logic
	customerType := r.FormValue("customer_type") // "new" or "existing"
	customerID := r.FormValue("customer_id")

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

	query := `INSERT INTO documents (user_id, filename, filepath, original_name, status, customer_id) 
	          VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`
	err = database.DB.QueryRow(query, userID, newFilename, filePath, handler.Filename, "uploaded", cID).Scan(&docID)
	
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
	
	// Phase 4: Trigger AI Classification
	go classifyDocumentWithAI(docID, result.Text)
}

func classifyDocumentWithAI(docID int, ocrText string) {
	fmt.Printf("Starting AI classification for document %d\n", docID)
	
	prompt := fmt.Sprintf(`Extract the document type and person's name from the following OCR text.
Return ONLY a valid JSON object with no markdown formatting and no extra text.
Format: {"document_type": "...", "person_name": "..."}
If you cannot find the name, use null.
If you cannot determine the type, use "Unknown".

OCR Text:
%s`, ocrText)

	requestBody, _ := json.Marshal(map[string]interface{}{
		"model":  "qwen2.5",
		"prompt": prompt,
		"stream": false,
		"format": "json", // Instruct Ollama to return JSON (works for newer models)
	})

	req, err := http.NewRequest("POST", "http://127.0.0.1:11434/api/generate", bytes.NewBuffer(requestBody))
	if err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to create AI request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to reach Ollama AI service: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		updateStatusAndError(docID, "failed", fmt.Sprintf("Ollama error: %s", string(respBody)))
		return
	}

	var ollamaResp struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to decode Ollama response: %v", err))
		return
	}

	// Parse the JSON string inside the response
	var extractedData struct {
		DocumentType string  `json:"document_type"`
		PersonName   *string `json:"person_name"`
	}
	
	// Try to unmarshal the response string
	if err := json.Unmarshal([]byte(ollamaResp.Response), &extractedData); err != nil {
		updateStatusAndError(docID, "failed", fmt.Sprintf("Failed to parse extracted JSON: %v. Raw: %s", err, ollamaResp.Response))
		return
	}

	// We will NOT auto-create the customer here anymore (Security Requirement #9).
	// We just save the AI extracted data and set status to 'needs_review'.
	// The user will confirm and map the customer via the new /api/documents/{id}/confirm endpoint.
	_, err = database.DB.Exec(`
		UPDATE documents 
		SET document_type = $1, person_name = $2, status = 'needs_review' 
		WHERE id = $3
	`, extractedData.DocumentType, extractedData.PersonName, docID)
	
	if err != nil {
		fmt.Printf("Failed to update database with AI results for doc %d: %v\n", docID, err)
		return
	}
	
	fmt.Printf("Successfully classified document %d (needs review): Type=%s, Person=%v\n", docID, extractedData.DocumentType, extractedData.PersonName)
}

func updateStatusAndError(docID int, status, errMsg string) {
	fmt.Printf("Document %d error: %s\n", docID, errMsg)
	database.DB.Exec("UPDATE documents SET status = $1 WHERE id = $2", status, docID)
}
