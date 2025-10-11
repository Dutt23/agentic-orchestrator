package cache

import (
	"context"
	"sync"
	"time"

	"github.com/lyzr/orchestrator/common/logger"
)

// Cache interface for key-value storage
type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Close() error
}

// MemoryCache is an in-memory cache implementation for MVP
type MemoryCache struct {
	data map[string]*cacheEntry
	mu   sync.RWMutex
	log  *logger.Logger
}

type cacheEntry struct {
	value     []byte
	expiresAt time.Time
}

// NewMemoryCache creates a new in-memory cache
func NewMemoryCache(log *logger.Logger) *MemoryCache {
	c := &MemoryCache{
		data: make(map[string]*cacheEntry),
		log:  log,
	}

	// Start cleanup goroutine
	go c.cleanup()

	return c
}

// Get retrieves a value from cache
func (c *MemoryCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[key]
	if !exists {
		return nil, false, nil
	}

	// Check expiration
	if time.Now().After(entry.expiresAt) {
		return nil, false, nil
	}

	return entry.value, true, nil
}

// Set stores a value in cache with TTL
func (c *MemoryCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data[key] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Delete removes a value from cache
func (c *MemoryCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data, key)
	return nil
}

// Close closes the cache (for interface compatibility)
func (c *MemoryCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data = nil
	c.log.Info("memory cache closed")
	return nil
}

// cleanup removes expired entries periodically
func (c *MemoryCache) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.data {
			if now.After(entry.expiresAt) {
				delete(c.data, key)
			}
		}
		c.mu.Unlock()
	}
}

// Stats returns cache statistics
func (c *MemoryCache) Stats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]interface{}{
		"entries": len(c.data),
		"type":    "memory",
	}
}
