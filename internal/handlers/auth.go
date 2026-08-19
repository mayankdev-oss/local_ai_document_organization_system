package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"docunest/internal/database"
	"github.com/alexedwards/argon2id"
	"github.com/golang-jwt/jwt/v5"
)

var jwtKey []byte

// InitAuth must be called from main.go after godotenv.Load()
func InitAuth() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" || secret == "your_jwt_secret_key" || secret == "CHANGE_ME_use_openssl_rand_base64_32" {
		log.Fatal("FATAL: JWT_SECRET environment variable is not set or is using a default value. Set a strong random secret before starting the server.")
	}
	jwtKey = []byte(secret)
}

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

const (
	UserIDKey contextKey = "user_id"
	RoleKey   contextKey = "role"
)

// --- Login Brute-Force Rate Limiter ---

type loginAttempt struct {
	count    int
	lockout  time.Time
	lastSeen time.Time
}

var (
	loginAttempts   = make(map[string]*loginAttempt)
	loginMutex      sync.Mutex
	maxLoginFails   = 10
	lockoutDuration = 15 * time.Minute
	attemptWindow   = 10 * time.Minute
)

func checkLoginRateLimit(ip string) bool {
	loginMutex.Lock()
	defer loginMutex.Unlock()

	now := time.Now()
	a, exists := loginAttempts[ip]
	if !exists {
		loginAttempts[ip] = &loginAttempt{count: 0, lastSeen: now}
		return true
	}

	// Reset if attempt window has passed
	if now.Sub(a.lastSeen) > attemptWindow {
		a.count = 0
		a.lockout = time.Time{}
	}

	// Check if locked out
	if !a.lockout.IsZero() && now.Before(a.lockout) {
		return false
	}

	a.lastSeen = now
	return true
}

func recordFailedLogin(ip string) {
	loginMutex.Lock()
	defer loginMutex.Unlock()

	a, exists := loginAttempts[ip]
	if !exists {
		loginAttempts[ip] = &loginAttempt{count: 1, lastSeen: time.Now()}
		return
	}
	a.count++
	a.lastSeen = time.Now()
	if a.count >= maxLoginFails {
		a.lockout = time.Now().Add(lockoutDuration)
	}
}

func clearLoginAttempts(ip string) {
	loginMutex.Lock()
	defer loginMutex.Unlock()
	delete(loginAttempts, ip)
}

// SeedAdminUser only inserts the admin if no users exist.
// It does NOT overwrite existing passwords on every startup.
func SeedAdminUser() {
	var count int
	err := database.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if err != nil {
		log.Printf("Error checking users count: %v", err)
		return
	}

	if count == 0 {
		hash, err := argon2id.CreateHash("admin", argon2id.DefaultParams)
		if err != nil {
			log.Printf("Error hashing password: %v", err)
			return
		}
		_, err = database.DB.Exec("INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'admin')", "admin", hash)
		if err != nil {
			log.Printf("Error seeding admin user: %v", err)
			return
		}
		log.Println("Seeded default admin user. IMPORTANT: Change the default password immediately.")
	} else {
		// Ensure the admin user has the admin role (for migrations)
		database.DB.Exec("UPDATE users SET role = 'admin' WHERE username = 'admin'")
	}

	// Seed test users
	seedTestUser("user1", "password123")
	seedTestUser("user2", "password123")
}

func seedTestUser(username, password string) {
	var count int
	database.DB.QueryRow("SELECT COUNT(*) FROM users WHERE username = $1", username).Scan(&count)
	if count == 0 {
		hash, _ := argon2id.CreateHash(password, argon2id.DefaultParams)
		database.DB.Exec("INSERT INTO users (username, password_hash, role) VALUES ($1, $2, 'user')", username, hash)
		log.Printf("Seeded test user: %s", username)
	}
}

func Login(w http.ResponseWriter, r *http.Request) {
	clientIP := r.RemoteAddr

	if !checkLoginRateLimit(clientIP) {
		http.Error(w, "Too many failed attempts. Please wait 15 minutes.", http.StatusTooManyRequests)
		return
	}

	var creds Credentials
	err := json.NewDecoder(r.Body).Decode(&creds)
	if err != nil {
		http.Error(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	// Validate input lengths to prevent oversized payloads
	if len(creds.Username) == 0 || len(creds.Username) > 50 || len(creds.Password) == 0 || len(creds.Password) > 128 {
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	var storedHash string
	var userID int
	var isDisabled bool
	err = database.DB.QueryRow("SELECT id, password_hash, is_disabled FROM users WHERE username = $1", creds.Username).Scan(&userID, &storedHash, &isDisabled)
	if err != nil {
		if err == sql.ErrNoRows {
			// Use constant-time comparison path (avoid timing oracle: still call ComparePasswordAndHash)
			argon2id.ComparePasswordAndHash(creds.Password, "$argon2id$v=19$m=65536,t=1,p=2$deadbeef$deadbeef")
			recordFailedLogin(clientIP)
			http.Error(w, "Invalid username or password", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if isDisabled {
		http.Error(w, "Account is disabled", http.StatusForbidden)
		return
	}

	match, err := argon2id.ComparePasswordAndHash(creds.Password, storedHash)
	if err != nil || !match {
		recordFailedLogin(clientIP)
		http.Error(w, "Invalid username or password", http.StatusUnauthorized)
		return
	}

	clearLoginAttempts(clientIP)

	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   userID,
		Username: creds.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Login successful"})
}

// Logout invalidates the session by expiring the cookie.
func Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "Logged out"})
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
			// Enforce HMAC signing method — prevents algorithm confusion attacks
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtKey, nil
		})

		if err != nil || !tkn.Valid {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
		
		var role string
		err = database.DB.QueryRow("SELECT role FROM users WHERE id = $1", claims.UserID).Scan(&role)
		if err == nil {
			ctx = context.WithValue(ctx, RoleKey, role)
		} else {
			ctx = context.WithValue(ctx, RoleKey, "user") // default
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SecurityHeaders adds essential HTTP security headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		// CSP: allow same-origin resources + trusted CDNs used by the app
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.tailwindcss.com https://cdn.jsdelivr.net; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"img-src 'self' data:; "+
				"frame-src 'self'; "+
				"connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
