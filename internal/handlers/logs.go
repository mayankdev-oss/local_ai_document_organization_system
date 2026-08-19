package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"docunest/internal/database"
)

// LogEvent structure for SSE
type SSELogEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	UserID    int                    `json:"user_id"`
	Action    string                 `json:"action"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

var (
	logClients = make(map[chan SSELogEvent]bool)
	logMutex   sync.Mutex
)

// LogEvent saves an event to the database and broadcasts it to SSE clients.
func LogEvent(userID int, action string, details map[string]interface{}) {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		log.Printf("Failed to marshal log details: %v", err)
		detailsJSON = []byte("{}")
	}

	_, err = database.DB.Exec("INSERT INTO audit_logs (user_id, action, details) VALUES ($1, $2, $3)", userID, action, string(detailsJSON))
	if err != nil {
		log.Printf("Failed to insert audit log: %v", err)
	}

	event := SSELogEvent{
		Timestamp: time.Now(),
		UserID:    userID,
		Action:    action,
		Details:   details,
	}

	logMutex.Lock()
	for client := range logClients {
		// Non-blocking send
		select {
		case client <- event:
		default:
			// If channel is full, we don't block
		}
	}
	logMutex.Unlock()
}

// StreamLogs handles SSE connections for live activity logs.
func StreamLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan SSELogEvent, 100)

	logMutex.Lock()
	logClients[clientChan] = true
	logMutex.Unlock()

	defer func() {
		logMutex.Lock()
		delete(logClients, clientChan)
		close(clientChan)
		logMutex.Unlock()
	}()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case event := <-clientChan:
			eventData, err := json.Marshal(event)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", eventData)
				w.Header().Set("X-Accel-Buffering", "no")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}
	}
}
