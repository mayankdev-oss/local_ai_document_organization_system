package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"docunest/internal/database"
	"github.com/gorilla/mux"
)

// Generate secure random token
func generateToken(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("Failed to generate random token: %v", err)
	}
	return hex.EncodeToString(b)
}

func CreateShareLink(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	docID := vars["id"]

	role, _ := r.Context().Value(RoleKey).(string)

	if role != "admin" {
		var exists bool
		err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM documents WHERE id = $1 AND user_id = $2)", docID, userID).Scan(&exists)
		if err != nil || !exists {
			http.Error(w, "Document not found or access denied", http.StatusNotFound)
			return
		}
	} else {
		var exists bool
		err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM documents WHERE id = $1)", docID).Scan(&exists)
		if err != nil || !exists {
			http.Error(w, "Document not found", http.StatusNotFound)
			return
		}
	}

	var req struct {
		ExpiresInHours int  `json:"expires_in_hours"`
		SingleUse      bool `json:"single_use"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.ExpiresInHours = 1 // default 1 hour
	}
	if req.ExpiresInHours <= 0 || req.ExpiresInHours > 72 {
		req.ExpiresInHours = 1
	}

	token := generateToken(32)
	expiresAt := time.Now().Add(time.Duration(req.ExpiresInHours) * time.Hour)

	_, err := database.DB.Exec(`
		INSERT INTO document_shares (token, document_id, expires_at, single_use)
		VALUES ($1, $2, $3, $4)
	`, token, docID, expiresAt, req.SingleUse)

	if err != nil {
		http.Error(w, "Failed to create share link", http.StatusInternalServerError)
		return
	}

	LogEvent(userID, "share_link_created", map[string]interface{}{"document_id": docID, "expires_at": expiresAt})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"expires_at": expiresAt,
		"url":        "/api/share/" + token,
	})
}

func ViewSharedDocument(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	token := vars["token"]

	var docID int
	var storedPath string
	var expiresAt time.Time
	var singleUse, isRevoked bool

	err := database.DB.QueryRow(`
		SELECT s.document_id, s.expires_at, s.single_use, s.is_revoked, d.filepath
		FROM document_shares s
		JOIN documents d ON s.document_id = d.id
		WHERE s.token = $1
	`, token).Scan(&docID, &expiresAt, &singleUse, &isRevoked, &storedPath)

	if err != nil {
		http.Error(w, "Invalid or expired share link", http.StatusNotFound)
		return
	}

	if isRevoked {
		http.Error(w, "This share link has been revoked", http.StatusForbidden)
		return
	}

	if time.Now().After(expiresAt) {
		http.Error(w, "This share link has expired", http.StatusForbidden)
		return
	}

	if singleUse {
		// Revoke it immediately
		database.DB.Exec("UPDATE document_shares SET is_revoked = TRUE WHERE token = $1", token)
	}

	// Serve inline for browser preview; X-Content-Type-Options is set by middleware
	absPath, err := filepath.Abs(storedPath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	absUploads, _ := filepath.Abs("./uploads")
	if len(absPath) <= len(absUploads) || absPath[:len(absUploads)] != absUploads {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// We don't have userID here since it's unauthenticated, so we log as system or track by token
	// Let's just log it using the document's owner
	var ownerID int
	database.DB.QueryRow("SELECT user_id FROM documents WHERE id = $1", docID).Scan(&ownerID)
	LogEvent(ownerID, "share_link_accessed", map[string]interface{}{"document_id": docID, "token_prefix": token[:8]})

	w.Header().Set("Content-Disposition", "inline")
	http.ServeFile(w, r, absPath)
}

func RevokeShareLink(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(UserIDKey).(int)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	docID := vars["id"]
	token := vars["token"]

	role, _ := r.Context().Value(RoleKey).(string)

	if role != "admin" {
		var exists bool
		err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM documents WHERE id = $1 AND user_id = $2)", docID, userID).Scan(&exists)
		if err != nil || !exists {
			http.Error(w, "Document not found or access denied", http.StatusNotFound)
			return
		}
	} else {
		var exists bool
		err := database.DB.QueryRow("SELECT EXISTS(SELECT 1 FROM documents WHERE id = $1)", docID).Scan(&exists)
		if err != nil || !exists {
			http.Error(w, "Document not found", http.StatusNotFound)
			return
		}
	}

	res, err := database.DB.Exec("UPDATE document_shares SET is_revoked = TRUE WHERE token = $1 AND document_id = $2", token, docID)
	if err != nil {
		http.Error(w, "Failed to revoke link", http.StatusInternalServerError)
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		http.Error(w, "Share link not found", http.StatusNotFound)
		return
	}

	LogEvent(userID, "share_link_revoked", map[string]interface{}{"document_id": docID, "token_prefix": token[:8]})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "Share link revoked successfully"})
}
