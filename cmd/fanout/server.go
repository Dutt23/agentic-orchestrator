package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now (TODO: Configure CORS properly in production)
		return true
	},
}

// Server handles WebSocket connections
type Server struct {
	hub *Hub
}

// NewServer creates a new Server instance
func NewServer(hub *Hub) *Server {
	return &Server{
		hub: hub,
	}
}

// HandleWebSocket handles WebSocket upgrade and registration
// URL: /ws?username=test-user
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Extract username from query parameter
	username := r.URL.Query().Get("username")
	if username == "" {
		http.Error(w, "username query parameter required", http.StatusBadRequest)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	// Create client
	client := NewClient(s.hub, conn, username)

	// Register client with hub
	s.hub.register <- client

	log.Printf("New WebSocket connection: username=%s, remote=%s", username, r.RemoteAddr)

	// Start client goroutines
	go client.writePump()
	go client.readPump()
}
