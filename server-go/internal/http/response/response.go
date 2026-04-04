package response

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

func Error(c *echo.Context, status int, message string) error {
	return c.JSON(status, map[string]any{
		"success": false,
		"error":   message,
	})
}

func Unauthorized(c *echo.Context, message string) error {
	return Error(c, http.StatusUnauthorized, message)
}
