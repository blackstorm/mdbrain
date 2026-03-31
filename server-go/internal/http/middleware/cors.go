package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

func ObsidianCORSMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			path := c.Path()
			if path == "" {
				path = c.Request().URL.Path
			}
			if !strings.HasPrefix(path, "/obsidian/") {
				return next(c)
			}
			headers := c.Response().Header()
			headers.Set("Access-Control-Allow-Origin", "*")
			headers.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			headers.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			if c.Request().Method == http.MethodOptions {
				return c.NoContent(http.StatusOK)
			}
			return next(c)
		}
	}
}
