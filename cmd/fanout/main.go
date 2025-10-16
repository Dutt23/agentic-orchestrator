package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

func main() {
	log.Println("Fanout Service starting...")

	// Get configuration from environment
	redisHost := getEnv("REDIS_HOST", "localhost")
	redisPort := getEnv("REDIS_PORT", "6379")
	port := getEnv("PORT", "8084")

	// Initialize Redis
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", redisHost, redisPort),
		Password: getEnv("REDIS_PASSWORD", ""),
		DB:       0,
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	log.Printf("Connected to Redis at %s:%s", redisHost, redisPort)

	// Create Hub (connection manager)
	hub := NewHub()
	go hub.Run()

	// Create Redis subscriber
	subscriber := NewRedisSubscriber(redisClient, hub)
	go subscriber.Start(ctx)

	// Create HTTP server with WebSocket handler
	server := NewServer(hub, redisClient)

	// Setup HTTP routes
	http.HandleFunc("/ws", server.HandleWebSocket)
	http.HandleFunc("/api/approval", server.HandleApproval)
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Start HTTP server
	addr := fmt.Sprintf(":%s", port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: http.DefaultServeMux,
		// No timeouts - WebSocket connections are long-lived
		// Timeouts would kill active connections
		ReadTimeout:  0,
		WriteTimeout: 0,
		// Optional: Set IdleTimeout for non-WebSocket connections
		IdleTimeout: 120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Fanout service listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down fanout service...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Fanout service stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
