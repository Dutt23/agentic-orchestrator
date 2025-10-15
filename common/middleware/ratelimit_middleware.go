package middleware

import (
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
	"github.com/lyzr/orchestrator/common/ratelimit"
)

// isInternalRequest checks if the request is from an internal service
// Internal services set X-Internal-Service header to bypass rate limits
func isInternalRequest(c echo.Context) bool {
	internalHeader := c.Request().Header.Get("X-Internal-Service")
	if internalHeader == "" {
		return false
	}

	// Verify against shared secret (prevents spoofing)
	// In production, use a proper secret management system
	expectedSecret := os.Getenv("INTERNAL_SERVICE_SECRET")
	if expectedSecret == "" {
		expectedSecret = "default-internal-secret-change-in-prod" // Fallback for dev
	}

	return internalHeader == expectedSecret
}

// GlobalRateLimitMiddleware checks the global service-wide rate limit
// Protects the entire service from being overwhelmed
// Skips rate limiting for internal service-to-service calls
func GlobalRateLimitMiddleware(rateLimiter *ratelimit.RateLimiter, limit int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip rate limiting for internal service calls
			if isInternalRequest(c) {
				return next(c)
			}

			result, err := rateLimiter.CheckGlobalLimit(c.Request().Context(), limit)
			if err != nil {
				// On error, allow request (fail open for availability)
				return next(c)
			}

			if !result.Allowed {
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"error":   "global_rate_limit_exceeded",
					"message": "Service is experiencing high load. Please try again later.",
					"details": map[string]interface{}{
						"limit":               result.Limit,
						"window":              "60 seconds",
						"retry_after_seconds": result.RetryAfterSeconds,
					},
				})
			}

			return next(c)
		}
	}
}

// UserRateLimitMiddleware checks per-user rate limits
// Requires username to be set in context by ExtractUsername middleware
// Skips rate limiting for internal service-to-service calls
func UserRateLimitMiddleware(rateLimiter *ratelimit.RateLimiter, limit int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Skip rate limiting for internal service calls
			// Internal services set X-Internal-Service header to bypass rate limits
			if isInternalRequest(c) {
				return next(c)
			}

			// Get username from context (set by ExtractUsername middleware)
			username, ok := c.Get("username").(string)
			if !ok || username == "" {
				// No username, skip rate limiting (or reject based on your policy)
				return next(c)
			}

			result, err := rateLimiter.CheckUserLimit(c.Request().Context(), username, limit, 60)
			if err != nil {
				// On error, allow request (fail open for availability)
				return next(c)
			}

			if !result.Allowed {
				return c.JSON(http.StatusTooManyRequests, map[string]interface{}{
					"error":   "user_rate_limit_exceeded",
					"message": "You have exceeded your request quota. Please wait before trying again.",
					"details": map[string]interface{}{
						"username":            username,
						"limit":               result.Limit,
						"window":              "60 seconds",
						"current_count":       result.CurrentCount,
						"retry_after_seconds": result.RetryAfterSeconds,
					},
				})
			}

			return next(c)
		}
	}
}
