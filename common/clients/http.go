package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	client    *http.Client
	logger    Logger
	useMover  bool
	moverConn *MoverCASClient // Reuse mover connection for HTTP
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
	req.Header.Set("X-Internal-Service", "test")

	// Extract user ID from context and set X-User-ID header
	if userID, ok := GetUserID(ctx); ok {
		req.Header.Set("X-User-ID", userID)
		c.logger.Debug("added X-User-ID header from context", "user_id", userID)
	}

	// Extract test token from context and set X-Test-Token header (for test endpoints)
	if testToken, ok := GetTestToken(ctx); ok {
		req.Header.Set("X-Test-Token", testToken)
		c.logger.Debug("added X-Test-Token header from context", "token", testToken)
	}

	// Execute request
	return c.client.Do(req)
}

// doRequestViaMover routes HTTP through mover service using zero-copy splice
// The calling service has NO IDEA this is happening - completely transparent!
func (c *HTTPClient) doRequestViaMover(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	c.logger.Debug("Routing HTTP request through mover (zero-copy splice)", "method", method, "url", url)

	// Parse URL to extract host and port
	parsedURL, err := parseURL(url)
	if err != nil {
		c.logger.Warn("Failed to parse URL for mover, falling back to direct", "error", err)
		return c.doRequestDirect(ctx, method, url, body)
	}

	// Prepare headers from context
	headers := make(map[string]string)

	// Extract user ID from context
	if userID, ok := GetUserID(ctx); ok {
		headers["X-User-ID"] = userID
	}

	// Extract test token from context
	if testToken, ok := GetTestToken(ctx); ok {
		headers["X-Test-Token"] = testToken
	}

	// Add internal service marker
	headers["X-Internal-Service"] = "test"

	// Read body if present
	var bodyBytes []byte
	if body != nil {
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	// Send request through mover using zero-copy splice (with streaming protocol)
	respBytes, err := c.moverConn.ProxyHTTPZeroCopy(ctx, method, parsedURL.host, parsedURL.port, parsedURL.path, headers, bodyBytes)
	if err != nil {
		c.logger.Warn("Mover HTTP proxy failed, falling back to direct", "error", err)
		// Fall back to direct HTTP on error
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		return c.doRequestDirect(ctx, method, url, bodyReader)
	}

	c.logger.Debug("Mover HTTP proxy succeeded (zero-copy)", "response_size", len(respBytes))

	// Parse raw HTTP response
	resp, err := parseRawHTTPResponse(respBytes)
	if err != nil {
		c.logger.Warn("Failed to parse mover response, falling back to direct", "error", err)
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}
		return c.doRequestDirect(ctx, method, url, bodyReader)
	}

	return resp, nil
}

// parsedURL holds parsed URL components
type parsedURL struct {
	host string
	port uint16
	path string
}

// parseURL extracts host, port, and path from a URL string
func parseURL(urlStr string) (*parsedURL, error) {
	// Simple URL parsing - expecting http://host:port/path format
	var host string
	var port uint16
	var path string

	// Remove http:// or https:// prefix
	if len(urlStr) > 7 && urlStr[:7] == "http://" {
		urlStr = urlStr[7:]
	} else if len(urlStr) > 8 && urlStr[:8] == "https://" {
		urlStr = urlStr[8:]
	}

	// Find path separator
	pathIdx := bytes.IndexByte([]byte(urlStr), '/')
	if pathIdx == -1 {
		host = urlStr
		path = "/"
	} else {
		host = urlStr[:pathIdx]
		path = urlStr[pathIdx:]
	}

	// Extract port if present
	portIdx := bytes.IndexByte([]byte(host), ':')
	if portIdx == -1 {
		port = 80 // Default HTTP port
	} else {
		var err error
		portStr := host[portIdx+1:]
		var portInt int
		if _, err = fmt.Sscanf(portStr, "%d", &portInt); err != nil {
			return nil, fmt.Errorf("invalid port: %w", err)
		}
		port = uint16(portInt)
		host = host[:portIdx]
	}

	return &parsedURL{
		host: host,
		port: port,
		path: path,
	}, nil
}

// HttpMetadata represents HTTP request metadata for streaming protocol
type HttpMetadata struct {
	Host          string     `json:"host"`
	Port          uint16     `json:"port"`
	Method        string     `json:"method"`
	Path          string     `json:"path"`
	Headers       [][2]string `json:"headers"` // Array of [key, value] tuples to match Rust Vec<(String, String)>
	ContentLength uint64     `json:"content_length"`
}

// buildHttpMetadata constructs HTTP metadata in JSON format for streaming protocol
// This is sent separately from the body to enable zero-copy splice
func buildHttpMetadata(method, path, host string, port uint16, headers map[string]string, bodyLen uint64) ([]byte, error) {
	// Convert headers map to array of [key, value] tuples (matches Rust Vec<(String, String)>)
	headerArray := make([][2]string, 0, len(headers))
	for k, v := range headers {
		headerArray = append(headerArray, [2]string{k, v})
	}

	metadata := HttpMetadata{
		Host:          host,
		Port:          port,
		Method:        method,
		Path:          path,
		Headers:       headerArray,
		ContentLength: bodyLen,
	}

	return json.Marshal(metadata)
}

// buildRawHTTPRequest constructs a raw HTTP/1.1 request
func buildRawHTTPRequest(method, path, host string, headers map[string]string, body []byte) []byte {
	var buf bytes.Buffer

	// Request line
	fmt.Fprintf(&buf, "%s %s HTTP/1.1\r\n", method, path)

	// Host header (required for HTTP/1.1)
	fmt.Fprintf(&buf, "Host: %s\r\n", host)

	// Custom headers
	for k, v := range headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
	}

	// Content-Length if body present
	if len(body) > 0 {
		fmt.Fprintf(&buf, "Content-Length: %d\r\n", len(body))
	}

	// End of headers
	buf.WriteString("\r\n")

	// Body
	if len(body) > 0 {
		buf.Write(body)
	}

	return buf.Bytes()
}

// parseRawHTTPResponse parses a raw HTTP response into http.Response
func parseRawHTTPResponse(respBytes []byte) (*http.Response, error) {
	// Split headers and body
	headerEndIdx := bytes.Index(respBytes, []byte("\r\n\r\n"))
	if headerEndIdx == -1 {
		return nil, fmt.Errorf("invalid HTTP response: no header/body separator")
	}

	headerBytes := respBytes[:headerEndIdx]
	bodyBytes := respBytes[headerEndIdx+4:]

	// Parse status line
	lines := bytes.Split(headerBytes, []byte("\r\n"))
	if len(lines) == 0 {
		return nil, fmt.Errorf("invalid HTTP response: no status line")
	}

	statusLine := string(lines[0])
	var statusCode int
	var httpVersion string
	// Support both HTTP/1.0 and HTTP/1.1 responses
	if _, err := fmt.Sscanf(statusLine, "%s %d", &httpVersion, &statusCode); err != nil {
		return nil, fmt.Errorf("failed to parse status code: %w", err)
	}
	// Validate HTTP version
	if httpVersion != "HTTP/1.0" && httpVersion != "HTTP/1.1" {
		return nil, fmt.Errorf("invalid HTTP version: %s", httpVersion)
	}

	// Parse headers
	headers := make(http.Header)
	for i := 1; i < len(lines); i++ {
		line := string(lines[i])
		if line == "" {
			continue
		}

		colonIdx := bytes.IndexByte([]byte(line), ':')
		if colonIdx == -1 {
			continue
		}

		key := line[:colonIdx]
		value := line[colonIdx+1:]
		// Trim leading space from value
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}
		headers.Add(key, value)
	}

	// Build http.Response
	resp := &http.Response{
		Status:        statusLine,
		StatusCode:    statusCode,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Body:          io.NopCloser(bytes.NewReader(bodyBytes)),
		ContentLength: int64(len(bodyBytes)),
		Header:        headers,
	}

	return resp, nil
}
