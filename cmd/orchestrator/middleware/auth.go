package middleware

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// ContextKey is a custom type for context keys to avoid collisions
type ContextKey string

const (
	// UsernameKey is the context key for storing the authenticated username
	UsernameKey ContextKey = "username"
)

// ExtractUsername is a middleware that extracts the X-User-ID header
// and stores it in the request context.
//
// This enables tag namespacing where each user has their own namespace:
// - User provides: "main"
// - Stored as: "alice/main"
// - Displayed as: "main" with owner="alice"
//
// Usage:
//   e := echo.New()
//   e.Use(middleware.ExtractUsername())
//
// Accessing in handlers:
//   username := middleware.GetUsername(c)
func ExtractUsername() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Extract X-User-ID header
			username := c.Request().Header.Get("X-User-ID")

			// For now, allow empty username (backwards compatibility)
			// In the future, you can enforce: if username == "" { return 401 }
			if username != "" {
				// Store in context for handler access
				c.Set(string(UsernameKey), username)
			}

			return next(c)
		}
	}
}

// ExtractUsernameStrict is a stricter version that requires X-User-ID header
// Use this when you want to enforce authentication for all routes
func ExtractUsernameStrict() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			username := c.Request().Header.Get("X-User-ID")

			if username == "" {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "X-User-ID header is required",
				})
			}

			c.Set(string(UsernameKey), username)
			return next(c)
		}
	}
}

// GetUsername retrieves the username from the request context
// Returns empty string if not set
func GetUsername(c echo.Context) string {
	username := c.Get(string(UsernameKey))
	if username == nil {
		return ""
	}
	return username.(string)
}

// RequireUsername ensures a username exists in context
// Returns an error response if not found
func RequireUsername(c echo.Context) (string, error) {
	username := GetUsername(c)
	if username == "" {
		err := c.JSON(http.StatusUnauthorized, map[string]interface{}{
			"error": "authentication required (X-User-ID header missing)",
		})
		return "", err
	}
	return username, nil
}
