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

	role, _ := r.Context().Value(RoleKey).(string)
	isAdmin := (role == "admin")

	var err error
	if isAdmin {
		err = database.DB.QueryRow("SELECT COUNT(*) FROM documents").Scan(&stats.TotalDocuments)
	} else {
		err = database.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE user_id = $1", userID).Scan(&stats.TotalDocuments)
	}
	if err != nil {
		http.Error(w, "Failed to get total documents", http.StatusInternalServerError)
		return
	}

	if isAdmin {
		err = database.DB.QueryRow("SELECT COUNT(*) FROM customers").Scan(&stats.TotalCustomers)
	} else {
		err = database.DB.QueryRow("SELECT COUNT(*) FROM customers WHERE user_id = $1", userID).Scan(&stats.TotalCustomers)
	}
	if err != nil {
		http.Error(w, "Failed to get total customers", http.StatusInternalServerError)
		return
	}

	if isAdmin {
		err = database.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE DATE(created_at) = CURRENT_DATE").Scan(&stats.ProcessedToday)
	} else {
		err = database.DB.QueryRow("SELECT COUNT(*) FROM documents WHERE user_id = $1 AND DATE(created_at) = CURRENT_DATE", userID).Scan(&stats.ProcessedToday)
	}
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

	// Ensure this is an admin!
	var role string
	database.DB.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
	if role != "admin" {
		http.Error(w, "Forbidden: Only admins can wipe data", http.StatusForbidden)
		return
	}

	LogEvent(userID, "data_wipe_started", map[string]interface{}{"action": "wipe my data"})

	// 1. Collect all file paths before deleting DB records
	rows, err := database.DB.Query("SELECT filepath FROM documents")
	if err != nil {
		log.Printf("WipeDatabase: failed to fetch filepaths: %v", err)
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

	if _, err := tx.Exec("DELETE FROM document_shares"); err != nil {
		log.Printf("WipeDatabase: failed to delete document_shares: %v", err)
		http.Error(w, "Wipe failed during document_shares deletion", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec("DELETE FROM audit_logs"); err != nil {
		log.Printf("WipeDatabase: failed to delete audit_logs: %v", err)
		http.Error(w, "Wipe failed during audit_logs deletion", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec("DELETE FROM documents"); err != nil {
		log.Printf("WipeDatabase: failed to delete documents: %v", err)
		http.Error(w, "Wipe failed during documents deletion", http.StatusInternalServerError)
		return
	}
	if _, err := tx.Exec("DELETE FROM customers"); err != nil {
		log.Printf("WipeDatabase: failed to delete customers: %v", err)
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

	log.Printf("WipeDatabase: System wiped — %d files deleted, %d skipped", deleted, skipped)
	LogEvent(userID, "data_wipe_completed", map[string]interface{}{"files_deleted": deleted})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":       "All data has been permanently deleted",
		"files_deleted": deleted,
		"files_skipped": skipped,
	})
}

