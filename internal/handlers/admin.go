package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"docunest/internal/database"
	"docunest/internal/models"
	"github.com/alexedwards/argon2id"
	"github.com/gorilla/mux"
)

func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(UserIDKey).(int)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var role string
		err := database.DB.QueryRow("SELECT role FROM users WHERE id = $1", userID).Scan(&role)
		if err != nil || role != "admin" {
			http.Error(w, "Forbidden: Admins only", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), "role", role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := database.DB.Query("SELECT id, username, role, is_disabled, created_at FROM users ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, "Failed to fetch users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.IsDisabled, &u.CreatedAt); err != nil {
			http.Error(w, "Failed to parse user", http.StatusInternalServerError)
			return
		}
		users = append(users, u)
	}

	w.Header().Set("Content-Type", "application/json")
	if users == nil {
		users = []models.User{}
	}
	json.NewEncoder(w).Encode(users)
}

func CreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid payload", http.StatusBadRequest)
		return
	}

	if len(req.Username) < 3 || len(req.Password) < 6 {
		http.Error(w, "Username or password too short", http.StatusBadRequest)
		return
	}

	hash, err := argon2id.CreateHash(req.Password, argon2id.DefaultParams)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	var userID int
	err = database.DB.QueryRow(
		"INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'user') RETURNING id",
		req.Username, hash,
	).Scan(&userID)
	if err != nil {
		http.Error(w, "Failed to create user (username may already exist)", http.StatusConflict)
		return
	}

	adminID, _ := r.Context().Value(UserIDKey).(int)
	LogEvent(adminID, "user_created", map[string]interface{}{"created_user_id": userID, "username": req.Username})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "User created", "id": userID})
}

func DisableUser(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]

	var isD bool
	err := database.DB.QueryRow("SELECT is_disabled FROM users WHERE id = $1 AND role != 'admin'", userID).Scan(&isD)
	if err != nil {
		http.Error(w, "User not found or cannot disable an admin", http.StatusForbidden)
		return
	}

	_, err = database.DB.Exec("UPDATE users SET is_disabled = $1 WHERE id = $2", !isD, userID)
	if err != nil {
		http.Error(w, "Failed to update user", http.StatusInternalServerError)
		return
	}

	adminID, _ := r.Context().Value(UserIDKey).(int)
	LogEvent(adminID, "user_toggled_disable", map[string]interface{}{"target_user_id": userID, "now_disabled": !isD})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "User status updated", "is_disabled": !isD})
}

func ResetPassword(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["id"]

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Password) < 6 {
		http.Error(w, "Invalid payload or password too short", http.StatusBadRequest)
		return
	}

	hash, err := argon2id.CreateHash(req.Password, argon2id.DefaultParams)
	if err != nil {
		http.Error(w, "Failed to hash password", http.StatusInternalServerError)
		return
	}

	res, err := database.DB.Exec("UPDATE users SET password_hash = $1 WHERE id = $2 AND role != 'admin'", hash, userID)
	if err != nil {
		http.Error(w, "Failed to update password", http.StatusInternalServerError)
		return
	}
	
	if affected, _ := res.RowsAffected(); affected == 0 {
		http.Error(w, "User not found or cannot reset admin password", http.StatusForbidden)
		return
	}

	adminID, _ := r.Context().Value(UserIDKey).(int)
	LogEvent(adminID, "user_password_reset", map[string]interface{}{"target_user_id": userID})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"message": "Password reset successfully"})
}
