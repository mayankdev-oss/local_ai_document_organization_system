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
	// Load .env file if it exists
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, relying on environment variables")
	}

	// Initialize Database
	err = database.ConnectDB()
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	err = database.InitSchema()
	if err != nil {
		log.Fatalf("Failed to initialize database schema: %v", err)
	}

	handlers.SeedAdminUser()

	// Router setup
	r := mux.NewRouter()
	
	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/login", handlers.Login).Methods("POST")
	
	// Protected routes
	protected := api.PathPrefix("/").Subrouter()
	protected.Use(handlers.AuthMiddleware)
	protected.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	}).Methods("GET")
	protected.HandleFunc("/customers", handlers.GetCustomers).Methods("GET")
	protected.HandleFunc("/documents", handlers.GetDocuments).Methods("GET")
	protected.HandleFunc("/documents/upload", handlers.UploadDocument).Methods("POST")

	// Static files (Frontend)
	r.PathPrefix("/").Handler(http.StripPrefix("/", http.FileServer(http.Dir("./public"))))

	log.Println("Server listening on :8080")
	if err := http.ListenAndServe(":8080", r); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
