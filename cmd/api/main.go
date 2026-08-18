package main

import (
	"log"
	"net/http"

	"docunest/internal/database"
	"docunest/internal/handlers"
	"github.com/gorilla/mux"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if present (local development only)
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found — relying on environment variables")
	}

	// Initialize Auth (loads JWT_SECRET from environment)
	handlers.InitAuth()

	// Initialize Database
	if err := database.ConnectDB(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	if err := database.InitSchema(); err != nil {
		log.Fatalf("Failed to initialize database schema: %v", err)
	}

	handlers.SeedAdminUser()

	// Router setup
	r := mux.NewRouter()

	// Apply security headers to ALL responses, including static assets
	r.Use(handlers.SecurityHeaders)

	// Public routes (unauthenticated)
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/login", handlers.Login).Methods("POST")

	// Protected routes
	protected := api.PathPrefix("/").Subrouter()
	protected.Use(handlers.AuthMiddleware)
	protected.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}).Methods("GET")
	protected.HandleFunc("/logout", handlers.Logout).Methods("POST")
	protected.HandleFunc("/customers", handlers.GetCustomers).Methods("GET")
	protected.HandleFunc("/customers/{id}/documents", handlers.GetCustomerDocuments).Methods("GET")
	protected.HandleFunc("/documents", handlers.GetDocuments).Methods("GET")
	protected.HandleFunc("/documents/{id}/view", handlers.ViewDocument).Methods("GET")
	protected.HandleFunc("/documents/upload", handlers.UploadDocument).Methods("POST")
	protected.HandleFunc("/documents/{id}/confirm", handlers.ConfirmDocument).Methods("POST")
	protected.HandleFunc("/stats", handlers.GetStats).Methods("GET")
	protected.HandleFunc("/admin/wipe", handlers.WipeDatabase).Methods("POST")

	// Static files — served last, after all API routes are matched
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("./public"))))

	log.Println("Server listening on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
