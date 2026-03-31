package response

import (
	"net/http"

	"github.com/labstack/echo/v5"
)

func OK(c *echo.Context, body any) error {
	return c.JSON(http.StatusOK, body)
}

func Success(c *echo.Context, body map[string]any) error {
	body["success"] = true
	return c.JSON(http.StatusOK, body)
}

func Error(c *echo.Context, status int, message string) error {
	return c.JSON(status, map[string]any{
		"success": false,
		"error":   message,
	})
}

func Unauthorized(c *echo.Context, message string) error {
	return Error(c, http.StatusUnauthorized, message)
}

func BadRequest(c *echo.Context, message string) error {
	return Error(c, http.StatusBadRequest, message)
}

func HTML(c *echo.Context, status int, body string) error {
	return c.HTML(status, body)
}
