package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"docunest/internal/database"
	"docunest/internal/storage"
)

func GetStats(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var stats struct {
		TotalDocuments int `json:"total_documents"`
		TotalCustomers int `json:"total_customers"`
		ProcessedToday int `json:"processed_today"`
	}

	err := database.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE user_id = $1", userID).Scan(&stats.TotalDocuments)
	if err != nil {
		http.Error(w, "Failed to get total documents", http.StatusInternalServerError)
		return
	}

	err = database.DB.QueryRow("SELECT COUNT(*) FROM customers WHERE user_id = $1", userID).Scan(&stats.TotalCustomers)
	if err != nil {
		http.Error(w, "Failed to get total customers", http.StatusInternalServerError)
		return
	}

	err = database.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE user_id = $1 AND DATE(created_at) = CURRENT_DATE", userID).Scan(&stats.ProcessedToday)
	if err != nil {
		http.Error(w, "Failed to get today's documents", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// WipeDatabase deletes all data belonging to the authenticated user and removes
// their uploaded files from disk. This is irreversible.
// The caller must supply {"confirmation": "wipe my data"} in the request body.
func WipeDatabase(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Require an explicit confirmation phrase in the body
	r.Body = http.MaxBytesReader(w, r.Body, 512)
	var body struct {
		Confirmation string `json:"confirmation"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}
	if body.Confirmation != "wipe my data" {
		http.Error(w, "Confirmation phrase did not match", http.StatusBadRequest)
		return
	}

	// 1. Collect all file paths before deleting DB records
	rows, err := database.DB.Query("SELECT filepath FROM documents WHERE user_id = $1", userID)
	if err != nil {
		log.Printf("WipeDatabase: failed to fetch filepaths for user %d: %v", userID, err)
		http.Error(w, "Failed to initiate wipe", http.StatusInternalServerError)
		return
	}
	var filePaths []string
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err == nil {
			filePaths = append(filePaths, fp)
		}
	}
	rows.Close()

	// 2. Delete DB records in dependency order within a transaction
	tx, err := database.DB.Begin()
	if err != nil {
		http.Error(w, "Failed to start transaction", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM audit_logs WHERE user_id = $1", userID); err != nil {
		log.Printf("WipeDatabase: failed to delete audit_logs for user %d: %v", userID, err)
		http.Error(w, "Wipe failed during audit_logs deletion", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec("DELETE FROM documents WHERE user_id = $1", userID); err != nil {
		log.Printf("WipeDatabase: failed to delete documents for user %d: %v", userID, err)
		http.Error(w, "Wipe failed during documents deletion", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec("DELETE FROM customers WHERE user_id = $1", userID); err != nil {
		log.Printf("WipeDatabase: failed to delete customers for user %d: %v", userID, err)
		http.Error(w, "Wipe failed during customers deletion", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "Wipe failed at commit", http.StatusInternalServerError)
		return
	}

	// 3. Delete files from disk — after the transaction commits successfully
	absUploads, _ := filepath.Abs(storage.UploadDir)
	deleted, skipped := 0, 0
	for _, fp := range filePaths {
		abs, err := filepath.Abs(fp)
		if err != nil || len(abs) <= len(absUploads) || abs[:len(absUploads)] != absUploads {
			// Skip any path that doesn't resolve inside the uploads dir (safety guard)
			skipped++
			continue
		}
		if err := os.Remove(abs); err != nil {
			log.Printf("WipeDatabase: could not remove file %s: %v", abs, err)
			skipped++
		} else {
			deleted++
		}
	}

	log.Printf("WipeDatabase: user %d wiped — %d files deleted, %d skipped", userID, deleted, skipped)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":       "All data has been permanently deleted",
		"files_deleted": deleted,
		"files_skipped": skipped,
	})
}

