package clients

import "context"

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// UserIDKey is the context key for user ID (for X-User-ID header)
	UserIDKey contextKey = "user-id"

	// TestTokenKey is the context key for test token (for X-Test-Token header in test endpoints)
	TestTokenKey contextKey = "test-token"

	// Future context keys can be added here:
	// OrgIDKey     contextKey = "org-id"
	// RequestIDKey contextKey = "request-id"
	// TraceIDKey   contextKey = "trace-id"
)

// WithUserID adds a user ID to the context
// This will be automatically extracted and added as X-User-ID header in HTTP requests
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, UserIDKey, userID)
}

// GetUserID retrieves the user ID from context
// Returns the user ID and true if found, empty string and false otherwise
func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(UserIDKey).(string)
	return userID, ok && userID != ""
}

// WithTestToken adds a test token to the context
// This will be automatically extracted and added as X-Test-Token header in HTTP requests
func WithTestToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, TestTokenKey, token)
}

// GetTestToken retrieves the test token from context
// Returns the test token and true if found, empty string and false otherwise
func GetTestToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(TestTokenKey).(string)
	return token, ok && token != ""
}
