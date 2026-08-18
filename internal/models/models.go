package models

import "time"

type User struct {
	ID           int       `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

type Customer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Document struct {
	ID           int       `json:"id"`
	Filename     string    `json:"filename"`
	Filepath     string    `json:"-"`
	OriginalName string    `json:"original_name"`
	Status       string    `json:"status"` // uploaded, reading, identifying, organizing, completed, failed
	OCRText      *string   `json:"ocr_text,omitempty"`
	DocumentType *string   `json:"document_type,omitempty"`
	PersonName   *string   `json:"person_name,omitempty"`
	Confidence   *float64  `json:"confidence,omitempty"`
	CustomerID   *string   `json:"customer_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}
