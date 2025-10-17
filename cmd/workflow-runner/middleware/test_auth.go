package middleware

import (
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

// TestAuthMiddleware protects test endpoints in workflow-runner
// Requires X-Test-Token header to match PERF_TEST_TOKEN env var
func TestAuthMiddleware() echo.MiddlewareFunc {
	expectedToken := os.Getenv("PERF_TEST_TOKEN")
	if expectedToken == "" {
		expectedToken = "perf-test-unsafe-default-token"
	}

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			token := c.Request().Header.Get("X-Test-Token")

			if token == "" {
				return c.JSON(http.StatusUnauthorized, map[string]interface{}{
					"error": "Test endpoints require X-Test-Token header",
				})
			}

			if token != expectedToken {
				return c.JSON(http.StatusForbidden, map[string]interface{}{
					"error": "Invalid test token",
				})
			}

			return next(c)
		}
	}
}
