package queue

import (
	"context"
	"sync"

	"github.com/lyzr/orchestrator/common/logger"
)

// Queue interface for message passing
type Queue interface {
	Publish(ctx context.Context, topic string, key string, message []byte) error
	Subscribe(ctx context.Context, topic string, handler MessageHandler) error
	Close() error
}

// MessageHandler processes messages
type MessageHandler func(ctx context.Context, key string, value []byte) error

// MemoryQueue is an in-memory queue for MVP
type MemoryQueue struct {
	topics map[string]chan *Message
	mu     sync.RWMutex
	log    *logger.Logger
}

// Message represents a queue message
type Message struct {
	Topic string
	Key   string
	Value []byte
}

// NewMemoryQueue creates a new in-memory queue
func NewMemoryQueue(log *logger.Logger) *MemoryQueue {
	return &MemoryQueue{
		topics: make(map[string]chan *Message),
		log:    log,
	}
}

// Publish publishes a message to a topic
func (q *MemoryQueue) Publish(ctx context.Context, topic string, key string, message []byte) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	ch, exists := q.topics[topic]
	if !exists {
		ch = make(chan *Message, 1000) // Buffered channel
		q.topics[topic] = ch
	}

	msg := &Message{
		Topic: topic,
		Key:   key,
		Value: message,
	}

	select {
	case ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Channel full, log warning
		q.log.Warn("queue full", "topic", topic)
		return nil
	}
}

// Subscribe subscribes to a topic and processes messages
func (q *MemoryQueue) Subscribe(ctx context.Context, topic string, handler MessageHandler) error {
	q.mu.Lock()
	ch, exists := q.topics[topic]
	if !exists {
		ch = make(chan *Message, 1000)
		q.topics[topic] = ch
	}
	q.mu.Unlock()

	q.log.Info("subscribing to topic", "topic", topic)

	go func() {
		for {
			select {
			case <-ctx.Done():
				q.log.Info("subscription cancelled", "topic", topic)
				return
			case msg := <-ch:
				if err := handler(ctx, msg.Key, msg.Value); err != nil {
					q.log.Error("message handler error", "topic", topic, "key", msg.Key, "error", err)
				}
			}
		}
	}()

	return nil
}

// Close closes the queue
func (q *MemoryQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for topic, ch := range q.topics {
		close(ch)
		q.log.Info("closed topic", "topic", topic)
	}

	return nil
}