package middleware

import (
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

// TestAuthMiddleware protects test endpoints
// Requires X-Test-Token header to match PERF_TEST_TOKEN env var
// This prevents accidental usage in production
func TestAuthMiddleware() echo.MiddlewareFunc {
	expectedToken := os.Getenv("PERF_TEST_TOKEN")
	if expectedToken == "" {
		expectedToken = "perf-test-unsafe-default-token"
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Check for test token header
			token := c.Request().Header.Get("X-Test-Token")

			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Test endpoints require X-Test-Token header",
					"hint":  "Set PERF_TEST_TOKEN env var and pass as X-Test-Token header",
				})
			}

			if token != expectedToken {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error": "Invalid test token",
				})
			}

			// Token valid, proceed
			return next(c)
		}
	}
}
