package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/http/middleware"
	"mdbrain.dev/internal/http/response"
	"mdbrain.dev/internal/infra/repository"
	"mdbrain.dev/internal/templatex"
	"mdbrain.dev/internal/util/authcompat"
)

type ConsoleAuthHandler struct {
	repo     *repository.Repository
	renderer *templatex.Renderer
	sessions *middleware.SessionManager
}

func NewConsoleAuthHandler(repo *repository.Repository, renderer *templatex.Renderer, sessions *middleware.SessionManager) *ConsoleAuthHandler {
	return &ConsoleAuthHandler{repo: repo, renderer: renderer, sessions: sessions}
}

func (h *ConsoleAuthHandler) LoginPage(c *echo.Context) error {
	html, err := h.renderer.Render("templates/console/login.html", map[string]any{
		"csrf-token": c.Get("csrf_token"),
	})
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, html)
}

func (h *ConsoleAuthHandler) InitPage(c *echo.Context) error {
	html, err := h.renderer.Render("templates/console/init.html", map[string]any{
		"csrf-token": c.Get("csrf_token"),
	})
	if err != nil {
		return err
	}
	return c.HTML(http.StatusOK, html)
}

func (h *ConsoleAuthHandler) InitConsole(c *echo.Context) error {
	hasUser, err := h.repo.HasAnyUser(c.Request().Context())
	if err != nil {
		return err
	}
	if hasUser {
		return response.Error(c, http.StatusForbidden, "System already initialized")
	}

	username := strings.TrimSpace(c.FormValue("username"))
	password := c.FormValue("password")
	tenantName := strings.TrimSpace(firstNonEmpty(c.FormValue("tenant-name"), c.FormValue("tenantName")))
	if username == "" || password == "" || tenantName == "" {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Missing required fields"})
	}

	if user, err := h.repo.GetUserByUsername(c.Request().Context(), username); err == nil && user != nil {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Username already exists"})
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	passwordHash, err := authcompat.Derive(password)
	if err != nil {
		return err
	}
	tenantID := uuid.NewString()
	userID := uuid.NewString()
	if err := h.repo.CreateTenant(c.Request().Context(), tenantID, tenantName); err != nil {
		return err
	}
	if err := h.repo.CreateUser(c.Request().Context(), userID, tenantID, username, passwordHash); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{
		"success":   true,
		"tenant-id": tenantID,
		"user-id":   userID,
	})
}

func (h *ConsoleAuthHandler) Login(c *echo.Context) error {
	username := strings.TrimSpace(c.FormValue("username"))
	password := c.FormValue("password")
	user, err := h.repo.GetUserByUsername(c.Request().Context(), username)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Invalid username or password"})
		}
		return err
	}
	ok, err := authcompat.Verify(password, user.PasswordHash)
	if err != nil || !ok {
		return c.JSON(http.StatusOK, map[string]any{"success": false, "error": "Invalid username or password"})
	}

	session := middleware.Session{
		UserID:    user.ID,
		TenantID:  user.TenantID,
		CSRFToken: firstNonEmpty(anyString(c.Get("csrf_token")), uuid.NewString()),
	}
	if err := h.sessions.Save(c, session); err != nil {
		return err
	}
	c.Set("session", session)
	c.Set("session.user_id", session.UserID)
	c.Set("session.tenant_id", session.TenantID)
	return c.JSON(http.StatusOK, map[string]any{
		"success": true,
		"user": map[string]any{
			"id":        user.ID,
			"username":  user.Username,
			"tenant-id": user.TenantID,
		},
	})
}

func (h *ConsoleAuthHandler) Logout(c *echo.Context) error {
	h.sessions.Clear(c)
	c.Set("session", middleware.Session{})
	c.Set("session.user_id", "")
	c.Set("session.tenant_id", "")
	return c.Redirect(http.StatusFound, "/console/login")
}

func (h *ConsoleAuthHandler) ChangePassword(c *echo.Context) error {
	userID := strings.TrimSpace(anyString(c.Get("session.user_id")))
	if userID == "" {
		return response.Error(c, http.StatusUnauthorized, "Unauthorized")
	}
	current := firstNonEmpty(c.FormValue("current-password"), c.FormValue("currentPassword"))
	next := firstNonEmpty(c.FormValue("new-password"), c.FormValue("newPassword"))
	confirm := firstNonEmpty(c.FormValue("confirm-password"), c.FormValue("confirmPassword"))

	switch {
	case strings.TrimSpace(current) == "" || strings.TrimSpace(next) == "" || strings.TrimSpace(confirm) == "":
		return response.Error(c, http.StatusBadRequest, "Missing required fields")
	case next != confirm:
		return response.Error(c, http.StatusBadRequest, "New password confirmation does not match")
	case len(next) < 8:
		return response.Error(c, http.StatusBadRequest, "New password must be at least 8 characters")
	}

	user, err := h.repo.GetUserByID(c.Request().Context(), userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return response.Error(c, http.StatusNotFound, "User not found")
		}
		return err
	}
	ok, err := authcompat.Verify(current, user.PasswordHash)
	if err != nil || !ok {
		return response.Error(c, http.StatusBadRequest, "Current password is incorrect")
	}
	newHash, err := authcompat.Derive(next)
	if err != nil {
		return err
	}
	if err := h.repo.UpdateUserPassword(c.Request().Context(), userID, newHash); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, map[string]any{"success": true, "message": "Password updated"})
}
