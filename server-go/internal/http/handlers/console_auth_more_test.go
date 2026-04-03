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

func TestConsoleInitValidationAndAlreadyInitialized(t *testing.T) {
	_, repo, _ := setupTestCore(t)
	handler := NewConsoleAuthHandler(repo, nil, middleware.NewSessionManager([]byte("session-hash-key-for-tests-32bytes"), false))

	req := httptest.NewRequest(http.MethodPost, "/console/init", strings.NewReader("username=admin&password=password123"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.InitConsole(c); err != nil {
		t.Fatalf("init console: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	missingPayload := readJSONMap(t, rec.Body.Bytes())
	if missingPayload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", missingPayload)
	}

	tenantID := uuid.NewString()
	userID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	hash, err := authcompat.Derive("password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(context.Background(), userID, tenantID, "admin", hash); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodPost, "/console/init", strings.NewReader("username=another&password=password123&tenant-name=Acme"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	if err := handler.InitConsole(c); err != nil {
		t.Fatalf("init console: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestConsoleAuthPagesRender(t *testing.T) {
	cfg, repo, _, renderer := setupTestCoreWithRenderer(t)
	handler := NewConsoleAuthHandler(repo, renderer, middleware.NewSessionManager(cfg.SessionHashKey(), false))

	req := httptest.NewRequest(http.MethodGet, "/console/login", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("csrf_token", "csrf-login")
	if err := handler.LoginPage(c); err != nil {
		t.Fatalf("login page: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Login to console dashboard") {
		t.Fatalf("unexpected login page response: %d %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/console/init", nil)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("csrf_token", "csrf-init")
	if err := handler.InitPage(c); err != nil {
		t.Fatalf("init page: %v", err)
	}
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Welcome to Mdbrain") {
		t.Fatalf("unexpected init page response: %d %s", rec.Code, rec.Body.String())
	}
}

func TestConsoleLoginInvalidCredentials(t *testing.T) {
	_, repo, _ := setupTestCore(t)
	handler := NewConsoleAuthHandler(repo, nil, middleware.NewSessionManager([]byte("session-hash-key-for-tests-32bytes"), false))

	tenantID := uuid.NewString()
	userID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	hash, err := authcompat.Derive("password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(context.Background(), userID, tenantID, "admin", hash); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/console/login", strings.NewReader("username=admin&password=wrong"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("csrf_token", "csrf-1")
	if err := handler.Login(c); err != nil {
		t.Fatalf("login: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	payload := readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("expected no cookie for invalid login")
	}

	req = httptest.NewRequest(http.MethodPost, "/console/login", strings.NewReader("username=unknown&password=password123"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.Set("csrf_token", "csrf-2")
	if err := handler.Login(c); err != nil {
		t.Fatalf("login: %v", err)
	}
	payload = readJSONMap(t, rec.Body.Bytes())
	if payload["success"] != false {
		t.Fatalf("expected success=false, got: %#v", payload)
	}
}

func TestConsoleLogoutRedirectsAndClearsCookie(t *testing.T) {
	sessions := middleware.NewSessionManager([]byte("session-hash-key-for-tests-32bytes"), false)
	handler := NewConsoleAuthHandler(nil, nil, sessions)

	req := httptest.NewRequest(http.MethodPost, "/console/logout", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.Logout(c); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/console/login" {
		t.Fatalf("unexpected location: %s", got)
	}
	foundSessionCookie := false
	for _, cookie := range rec.Result().Cookies() {
		if cookie.Name == middleware.SessionCookieName {
			foundSessionCookie = true
			if cookie.MaxAge >= 0 {
				t.Fatalf("expected deleted session cookie, got maxAge=%d", cookie.MaxAge)
			}
		}
	}
	if !foundSessionCookie {
		t.Fatalf("expected session cookie to be set")
	}
}

func TestChangePasswordErrorPaths(t *testing.T) {
	_, repo, _ := setupTestCore(t)
	handler := NewConsoleAuthHandler(repo, nil, middleware.NewSessionManager([]byte("session-hash-key-for-tests-32bytes"), false))

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

	t.Run("unauthorized without session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/console/user/password", strings.NewReader("current-password=old-password&new-password=new-password&confirm-password=new-password"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		if err := handler.ChangePassword(c); err != nil {
			t.Fatalf("change password: %v", err)
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("confirmation mismatch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/console/user/password", strings.NewReader("current-password=old-password&new-password=abc12345&confirm-password=abc54321"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("session.user_id", userID)
		if err := handler.ChangePassword(c); err != nil {
			t.Fatalf("change password: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("short password", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/console/user/password", strings.NewReader("current-password=old-password&new-password=short&confirm-password=short"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("session.user_id", userID)
		if err := handler.ChangePassword(c); err != nil {
			t.Fatalf("change password: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("wrong current password", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/console/user/password", strings.NewReader("current-password=wrong&new-password=new-password&confirm-password=new-password"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("session.user_id", userID)
		if err := handler.ChangePassword(c); err != nil {
			t.Fatalf("change password: %v", err)
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("user not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/console/user/password", strings.NewReader("current-password=old-password&new-password=new-password&confirm-password=new-password"))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		rec := httptest.NewRecorder()
		c := echo.New().NewContext(req, rec)
		c.Set("session.user_id", uuid.NewString())
		if err := handler.ChangePassword(c); err != nil {
			t.Fatalf("change password: %v", err)
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
	})
}
