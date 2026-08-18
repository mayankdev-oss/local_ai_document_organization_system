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
	UserID    int       `json:"user_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Document struct {
	ID           int       `json:"id"`
	UserID       int       `json:"user_id"`
	Filename     string    `json:"filename"`
	Filepath     string    `json:"-"`
	OriginalName string    `json:"original_name"`
	Status       string    `json:"status"` // uploaded, reading, identifying, organizing, needs_review, completed, failed
	OCRText      *string   `json:"ocr_text,omitempty"`
	DocumentType *string   `json:"document_type,omitempty"`
	PersonName   *string   `json:"person_name,omitempty"`
	Confidence   *float64  `json:"confidence,omitempty"`
	CustomerID   *string   `json:"customer_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type AuditLog struct {
	ID         int       `json:"id"`
	UserID     int       `json:"user_id"`
	DocumentID int       `json:"document_id"`
	Action     string    `json:"action"`
	Details    *string   `json:"details,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
