package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now (TODO: Configure CORS properly in production)
		return true
	},
}

// Server handles WebSocket connections and approval requests
type Server struct {
	hub   *Hub
	redis *redis.Client
}

// NewServer creates a new Server instance
func NewServer(hub *Hub, redisClient *redis.Client) *Server {
	return &Server{
		hub:   hub,
		redis: redisClient,
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

// ApprovalRequest represents an approval decision from the user
type ApprovalRequest struct {
	RunID    string                 `json:"run_id"`
	NodeID   string                 `json:"node_id"`
	Approved bool                   `json:"approved"`
	Comment  string                 `json:"comment,omitempty"`
	Data     map[string]interface{} `json:"data,omitempty"`
}

// HandleApproval handles user approval decisions
// POST /api/approval
func (s *Server) HandleApproval(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract username from header (set by middleware or authentication)
	username := r.Header.Get("X-User-ID")
	if username == "" {
		http.Error(w, "X-User-ID header required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req ApprovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.RunID == "" || req.NodeID == "" {
		http.Error(w, "run_id and node_id are required", http.StatusBadRequest)
		return
	}

	log.Printf("Received approval decision: username=%s, run_id=%s, node_id=%s, approved=%v",
		username, req.RunID, req.NodeID, req.Approved)

	// Update approval status in Redis
	approvalKey := "hitl:approval:" + req.RunID + ":" + req.NodeID

	// Load existing approval request
	ctx := context.Background()
	data, err := s.redis.Get(ctx, approvalKey).Result()
	if err != nil {
		log.Printf("Failed to get approval request: %v", err)
		http.Error(w, "Approval request not found", http.StatusNotFound)
		return
	}

	// Parse existing approval request
	var approvalData map[string]interface{}
	if err := json.Unmarshal([]byte(data), &approvalData); err != nil {
		log.Printf("Failed to parse approval data: %v", err)
		http.Error(w, "Invalid approval data", http.StatusInternalServerError)
		return
	}

	// Update status and add approval metadata
	if req.Approved {
		approvalData["status"] = "approved"
	} else {
		approvalData["status"] = "rejected"
	}
	approvalData["approved_by"] = username
	approvalData["approved_at"] = time.Now().Unix()
	approvalData["comment"] = req.Comment

	// Merge any additional data from request
	if req.Data != nil {
		for k, v := range req.Data {
			approvalData[k] = v
		}
	}

	// Store updated approval data back to Redis
	updatedJSON, err := json.Marshal(approvalData)
	if err != nil {
		log.Printf("Failed to marshal updated approval data: %v", err)
		http.Error(w, "Failed to update approval", http.StatusInternalServerError)
		return
	}

	// Update Redis with same TTL (24 hours)
	if err := s.redis.Set(ctx, approvalKey, updatedJSON, 24*time.Hour).Err(); err != nil {
		log.Printf("Failed to update approval in Redis: %v", err)
		http.Error(w, "Failed to update approval", http.StatusInternalServerError)
		return
	}

	log.Printf("Approval updated successfully: run_id=%s, node_id=%s, status=%s",
		req.RunID, req.NodeID, approvalData["status"])

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Approval recorded successfully",
		"run_id":  req.RunID,
		"node_id": req.NodeID,
		"status":  approvalData["status"],
	})
}
