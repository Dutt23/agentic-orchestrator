package main

import (
	"log"
	"sync"
)

// Hub maintains active WebSocket connections and broadcasts messages
type Hub struct {
	// Map: username → []*Client
	connections map[string][]*Client
	mutex       sync.RWMutex

	// Channel for registering clients
	register chan *Client

	// Channel for unregistering clients
	unregister chan *Client

	// Channel for broadcasting messages
	broadcast chan *Message
}

// Message represents a message to be broadcast
type Message struct {
	Username string
	Data     []byte
}

// NewHub creates a new Hub instance
func NewHub() *Hub {
	return &Hub{
		connections: make(map[string][]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan *Message, 256),
	}
}

// Run starts the hub's main loop
func (h *Hub) Run() {
	log.Println("Hub started")

	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastToUsername(message)
		}
	}
}

// registerClient adds a client to the hub
func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	h.connections[client.username] = append(h.connections[client.username], client)
	log.Printf("Client registered: username=%s, total_for_user=%d", 
		client.username, len(h.connections[client.username]))
}

// unregisterClient removes a client from the hub
func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	clients := h.connections[client.username]
	for i, c := range clients {
		if c == client {
			// Remove client from slice
			h.connections[client.username] = append(clients[:i], clients[i+1:]...)
			close(client.send)
			
			// If no more clients for this user, remove the map entry
			if len(h.connections[client.username]) == 0 {
				delete(h.connections, client.username)
			}
			
			log.Printf("Client unregistered: username=%s, remaining_for_user=%d", 
				client.username, len(h.connections[client.username]))
			break
		}
	}
}

// broadcastToUsername sends a message to all connections for a specific username
func (h *Hub) broadcastToUsername(message *Message) {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients := h.connections[message.Username]
	if len(clients) == 0 {
		// No clients connected for this username, skip
		return
	}

	log.Printf("Broadcasting to username=%s, client_count=%d", 
		message.Username, len(clients))

	for _, client := range clients {
		select {
		case client.send <- message.Data:
			// Message sent successfully
		default:
			// Client's send buffer is full, close the connection
			log.Printf("Client send buffer full, closing connection: username=%s", client.username)
			close(client.send)
		}
	}
}

// GetConnectionCount returns the total number of active connections
func (h *Hub) GetConnectionCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	count := 0
	for _, clients := range h.connections {
		count += len(clients)
	}
	return count
}

// GetUserCount returns the number of unique users connected
func (h *Hub) GetUserCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	return len(h.connections)
}
