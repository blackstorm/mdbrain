package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/http/response"
	"mdbrain.dev/internal/infra/repository"
)

func SessionMiddleware(manager *SessionManager) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			session, err := manager.Load(c)
			if err != nil {
				return err
			}
			if err := manager.EnsureCSRF(&session); err != nil {
				return err
			}
			putSession(c, session)
			err = next(c)
			if err != nil {
				return err
			}
			return manager.Save(c, sessionFromContext(c))
		}
	}
}

func ConsoleCSRFMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if !strings.HasPrefix(c.Path(), "/console") {
				return next(c)
			}
			if !stateChangingMethod(c.Request().Method) {
				return next(c)
			}

			session := sessionFromContext(c)
			expected := strings.TrimSpace(session.CSRFToken)
			provided := parseCSRFToken(c)
			if expected == "" || provided == "" || expected != provided {
				return response.Error(c, http.StatusForbidden, "CSRF token missing or incorrect")
			}
			return next(c)
		}
	}
}

func ConsoleAuthMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if strings.TrimSpace(sessionUserID(c)) != "" {
				return next(c)
			}
			return redirect(c, "/console/login")
		}
	}
}

func ConsoleInitCheckMiddleware(repo *repository.Repository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			path := c.Path()
			if path == "" {
				path = c.Request().URL.Path
			}
			if !strings.HasPrefix(path, "/console") {
				return next(c)
			}
			if path == "/console/health" || path == "/console/domain-check" {
				return next(c)
			}

			hasUser, err := repo.HasAnyUser(c.Request().Context())
			if err != nil {
				return err
			}

			if hasUser && path == "/console/init" {
				return redirect(c, "/console/login")
			}
			if !hasUser && path != "/console/init" {
				return redirect(c, "/console/init")
			}
			return next(c)
		}
	}
}

func ConsoleNoIndexMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if err := next(c); err != nil {
				return err
			}
			c.Response().Header().Set("X-Robots-Tag", "noindex, nofollow")
			return nil
		}
	}
}
