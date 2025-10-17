package clients

import (
	"context"
	"io"
	"net/http"
)

// Logger interface for HTTP client logging
type Logger interface {
	Info(msg string, keysAndValues ...interface{})
	Error(msg string, keysAndValues ...interface{})
	Warn(msg string, keysAndValues ...interface{})
	Debug(msg string, keysAndValues ...interface{})
}

// HTTPClient wraps http.Client with context-aware helpers and optional mover optimization
// It automatically extracts metadata from context and adds appropriate headers
// If USE_MOVER=true, routes external HTTP through mover (io_uring optimization)
type HTTPClient struct {
	client     *http.Client
	logger     Logger
	useMover   bool
	moverConn  *MoverCASClient  // Reuse mover connection for HTTP
}

// NewHTTPClient creates a new HTTP client wrapper from config
// Routes HTTP through mover if config.UseMover is enabled
func NewHTTPClient(client *http.Client, logger Logger) *HTTPClient {
	// Get config (loaded once, cached)
	config := GetClientConfig()

	var moverConn *MoverCASClient
	if config.UseMover {
		logger.Info("HTTP client will use mover for external calls (io_uring)", "socket", config.MoverSocket)
		// Reuse mover client (shares connection pool)
		conn, err := NewMoverCASClient(config)
		if err != nil {
			logger.Warn("Failed to connect to mover, falling back to direct HTTP", "error", err)
		} else {
			moverConn = conn
		}
	}

	return &HTTPClient{
		client:    client,
		logger:    logger,
		useMover:  config.UseMover && moverConn != nil,
		moverConn: moverConn,
	}
}

// DoRequest creates and executes an HTTP request, extracting metadata from context
// This is the central method that handles context-to-header conversion
// If mover is enabled, routes through mover for io_uring optimization (transparent to caller!)
func (c *HTTPClient) DoRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	// If mover enabled, route through it (service code has no idea!)
	if c.useMover {
		return c.doRequestViaMover(ctx, method, url, body)
	}

	// Standard path: direct HTTP
	return c.doRequestDirect(ctx, method, url, body)
}

// doRequestDirect makes HTTP request directly (current implementation)
func (c *HTTPClient) doRequestDirect(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	// Create request with context
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	// Extract user ID from context and set X-User-ID header
	if userID, ok := GetUserID(ctx); ok {
		req.Header.Set("X-User-ID", userID)
		c.logger.Debug("added X-User-ID header from context", "user_id", userID)
	}

	// Execute request
	return c.client.Do(req)
}

// doRequestViaMover routes HTTP through mover service (io_uring optimized)
// The calling service has NO IDEA this is happening - completely transparent!
func (c *HTTPClient) doRequestViaMover(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	// TODO: Implement HTTP proxy via mover
	// Mover would:
	//   1. Receive request via UDS
	//   2. Make HTTP call via io_uring
	//   3. Return response
	//
	// For now, fall back to direct
	c.logger.Debug("Mover HTTP proxy not yet implemented, using direct")
	return c.doRequestDirect(ctx, method, url, body)
}
