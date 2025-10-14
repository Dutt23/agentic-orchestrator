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

// HTTPClient wraps http.Client with context-aware helpers
// It automatically extracts metadata from context and adds appropriate headers
type HTTPClient struct {
	client *http.Client
	logger Logger
}

// NewHTTPClient creates a new HTTP client wrapper
func NewHTTPClient(client *http.Client, logger Logger) *HTTPClient {
	return &HTTPClient{
		client: client,
		logger: logger,
	}
}

// DoRequest creates and executes an HTTP request, extracting metadata from context
// This is the central method that handles context-to-header conversion
func (c *HTTPClient) DoRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
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

	// Future: Extract more metadata from context and set headers
	// Example:
	// if orgID, ok := GetOrgID(ctx); ok {
	//     req.Header.Set("X-Org-ID", orgID)
	// }
	// if requestID, ok := GetRequestID(ctx); ok {
	//     req.Header.Set("X-Request-ID", requestID)
	// }

	// Execute request
	return c.client.Do(req)
}
