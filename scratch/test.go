package main

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	connStr := "user=postgres password=3020 dbname=postgres sslmode=disable host=localhost"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	var existingCustomerID *string
	// Use a known document with NULL customer_id (e.g. id 5)
	err = db.QueryRow("SELECT customer_id FROM documents WHERE id = 5").Scan(&existingCustomerID)
	if err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Printf("SUCCESS, existingCustomerID is %v\n", existingCustomerID)
	}
}
