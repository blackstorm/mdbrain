package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/http/middleware"
	"mdbrain.dev/internal/util/authcompat"
)

func TestConsoleInitAndLogin(t *testing.T) {
	cfg, repo, _ := setupTestCore(t)
	sessions := middleware.NewSessionManager(cfg.SessionHashKey(), false)
	handler := NewConsoleAuthHandler(repo, nil, sessions)

	req := httptest.NewRequest(http.MethodPost, "/console/init", strings.NewReader("username=admin&password=password123&tenant-name=Acme"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("csrf_token", "csrf-1")
	if err := handler.InitConsole(c); err != nil {
		t.Fatalf("init console: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("unexpected init response: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/console/login", strings.NewReader("username=admin&password=password123"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("csrf_token", "csrf-2")
	if err := handler.Login(c); err != nil {
		t.Fatalf("login: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("unexpected login response: %d %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie")
	}
}

func TestChangePassword(t *testing.T) {
	cfg, repo, _ := setupTestCore(t)
	sessions := middleware.NewSessionManager(cfg.SessionHashKey(), false)
	handler := NewConsoleAuthHandler(repo, nil, sessions)

	tenantID := uuid.NewString()
	userID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	hash, err := authcompat.Derive("old-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(context.Background(), userID, tenantID, "admin", hash); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/console/user/password", strings.NewReader("current-password=old-password&new-password=new-password&confirm-password=new-password"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.user_id", userID)
	if err := handler.ChangePassword(c); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("unexpected response: %d %s", rec.Code, rec.Body.String())
	}
}
