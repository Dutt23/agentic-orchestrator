package main

import (
	"context"
	"log"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisSubscriber listens to Redis PubSub and forwards messages to Hub
type RedisSubscriber struct {
	redis *redis.Client
	hub   *Hub
}

// NewRedisSubscriber creates a new RedisSubscriber instance
func NewRedisSubscriber(redisClient *redis.Client, hub *Hub) *RedisSubscriber {
	return &RedisSubscriber{
		redis: redisClient,
		hub:   hub,
	}
}

// Start begins listening to Redis PubSub channels
func (s *RedisSubscriber) Start(ctx context.Context) {
	// Subscribe to pattern: workflow:events:*
	// This allows us to receive events for all usernames
	pubsub := s.redis.PSubscribe(ctx, "workflow:events:*")
	defer pubsub.Close()

	log.Println("Redis subscriber started, listening to: workflow:events:*")

	// Wait for confirmation that subscription was successful
	_, err := pubsub.Receive(ctx)
	if err != nil {
		log.Fatalf("Failed to subscribe to Redis: %v", err)
	}

	log.Println("Redis subscription confirmed")

	// Listen for messages
	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			log.Println("Redis subscriber stopping")
			return

		case msg := <-ch:
			if msg == nil {
				continue
			}

			// Extract username from channel name
			// Channel format: workflow:events:{username}
			username := extractUsernameFromChannel(msg.Channel)
			if username == "" {
				log.Printf("Invalid channel format: %s", msg.Channel)
				continue
			}

			log.Printf("Received event for username=%s, size=%d bytes", username, len(msg.Payload))

			// Forward to hub
			s.hub.broadcast <- &Message{
				Username: username,
				Data:     []byte(msg.Payload),
			}
		}
	}
}

// extractUsernameFromChannel extracts username from channel name
// Example: "workflow:events:test-user" → "test-user"
func extractUsernameFromChannel(channel string) string {
	parts := strings.Split(channel, ":")
	if len(parts) != 3 || parts[0] != "workflow" || parts[1] != "events" {
		return ""
	}
	return parts[2]
}
