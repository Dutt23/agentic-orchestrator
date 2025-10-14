package clients

import "context"

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// UserIDKey is the context key for user ID (for X-User-ID header)
	UserIDKey contextKey = "user-id"

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
