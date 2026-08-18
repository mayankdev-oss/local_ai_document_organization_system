package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"docunest/internal/database"
	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
)

var jwtKey = []byte(getEnv("JWT_SECRET", "super_secret_key_for_docunest"))

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type contextKey string

const UserIDKey contextKey = "user_id"

func SeedAdminUser() {
	var count int
	err := database.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		log.Printf("Error checking users count: %v", err)
		return
	}

	hash, err := argon2id.CreateHash("admin", argon2id.DefaultParams)
	if err != nil {
		log.Printf("Error hashing password: %v", err)
		return
	}

	if count == 0 {
		_, err = database.DB.Exec("INSERT INTO users (username, password_hash) VALUES ($1, $2)", "admin", hash)
		if err != nil {
			log.Printf("Error seeding admin user: %v", err)
			return
		}
		log.Println("Seeded default admin user (admin/admin)")
	} else {
		// Temporary fix to ensure the existing admin gets the new Argon2id hash
		_, err = database.DB.Exec("UPDATE users SET password_hash = $1 WHERE username = 'admin'", hash)
		if err != nil {
			log.Printf("Error updating admin user: %v", err)
			return
		}
	}
}

func Login(w http.ResponseWriter, r *http.Request) {
	var creds Credentials
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	var storedHash string
	var userID int
	err = database.DB.QueryRow("SELECT id, password_hash FROM users WHERE username = $1", creds.Username).Scan(&userID, &storedHash)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	match, err := argon2id.ComparePasswordAndHash(creds.Password, storedHash)
	if err != nil || !match {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   userID,
		Username: creds.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    tokenString,
		Expires:  expirationTime,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})

	json.NewEncoder(w).Encode(map[string]string{"message": "Login successful"})
}

func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("token")
		if err != nil {
			if err == http.ErrNoCookie {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		tokenStr := cookie.Value
		claims := &Claims{}

		tkn, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
			return jwtKey, nil
		})

		if err != nil {
			if err == jwt.ErrSignatureInvalid {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		if !tkn.Valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Inject user_id into context
		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
