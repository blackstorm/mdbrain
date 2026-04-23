package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/config"
	"mdbrain.dev/internal/infra/db"
	"mdbrain.dev/internal/infra/repository"
)

func TestObsidianCORSMiddleware(t *testing.T) {
	e := echo.New()
	mw := ObsidianCORSMiddleware()

	called := false
	handler := mw(func(c *echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/obsidian/sync/changes", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := handler(c); err != nil {
		t.Fatalf("cors middleware: %v", err)
	}
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("unexpected response: status=%d called=%v", rec.Code, called)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("unexpected allow-origin: %s", got)
	}

	called = false
	req = httptest.NewRequest(http.MethodOptions, "/obsidian/sync/changes", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := handler(c); err != nil {
		t.Fatalf("cors middleware: %v", err)
	}
	if rec.Code != http.StatusOK || called {
		t.Fatalf("unexpected preflight response: status=%d called=%v", rec.Code, called)
	}

	called = false
	req = httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := handler(c); err != nil {
		t.Fatalf("cors middleware: %v", err)
	}
	if !called {
		t.Fatalf("expected next handler called")
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("did not expect cors header for non-obsidian path, got: %s", got)
	}
}

func TestSessionMiddleware(t *testing.T) {
	e := echo.New()
	manager := NewSessionManager([]byte("session-hash-key-for-tests-32bytes"), false)
	mw := SessionMiddleware(manager)

	t.Run("loads existing session, ensures csrf, and persists", func(t *testing.T) {
		encoded, err := manager.encode(Session{UserID: "user-1", TenantID: "tenant-1"})
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/console/vaults", nil)
		req.AddCookie(&http.Cookie{Name: SessionCookieName, Value: encoded})
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c *echo.Context) error {
			session := sessionFromContext(c)
			if session.UserID != "user-1" || session.TenantID != "tenant-1" {
				t.Fatalf("unexpected session in context: %#v", session)
			}
			if session.CSRFToken == "" {
				t.Fatalf("expected csrf token to be set")
			}
			session.TenantID = "tenant-2"
			putSession(c, session)
			return nil
		})

		if err := handler(c); err != nil {
			t.Fatalf("session middleware: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}

		var cookie *http.Cookie
		for _, item := range rec.Result().Cookies() {
			if item.Name == SessionCookieName {
				cookie = item
				break
			}
		}
		if cookie == nil {
			t.Fatalf("expected session cookie to be written")
		}
		decoded, err := manager.decode(cookie.Value)
		if err != nil {
			t.Fatalf("decode cookie: %v", err)
		}
		if decoded.UserID != "user-1" || decoded.TenantID != "tenant-2" || decoded.CSRFToken == "" {
			t.Fatalf("unexpected decoded session: %#v", decoded)
		}
	})

	t.Run("creates new session and csrf token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/console/login", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := mw(func(c *echo.Context) error {
			session := sessionFromContext(c)
			if session.CSRFToken == "" {
				t.Fatalf("expected csrf token to be created")
			}
			return nil
		})

		if err := handler(c); err != nil {
			t.Fatalf("session middleware: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rec.Code)
		}
	})
}

func TestConsoleCSRFMiddleware(t *testing.T) {
	e := echo.New()
	mw := ConsoleCSRFMiddleware()

	run := func(method, path, headerToken, formToken string, session Session) (status int, called bool) {
		var req *http.Request
		if formToken != "" {
			req = httptest.NewRequest(method, path, strings.NewReader("__anti-forgery-token="+formToken))
			req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		if headerToken != "" {
			req.Header.Set("X-CSRF-Token", headerToken)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath(path)
		putSession(c, session)

		handler := mw(func(c *echo.Context) error {
			called = true
			return c.NoContent(http.StatusOK)
		})
		if err := handler(c); err != nil {
			t.Fatalf("csrf middleware: %v", err)
		}
		return rec.Code, called
	}

	status, called := run(http.MethodPost, "/console/vaults", "", "", Session{CSRFToken: "token-1"})
	if status != http.StatusForbidden || called {
		t.Fatalf("expected csrf forbidden for missing token, got status=%d called=%v", status, called)
	}

	status, called = run(http.MethodPost, "/console/vaults", "wrong", "", Session{CSRFToken: "token-1"})
	if status != http.StatusForbidden || called {
		t.Fatalf("expected csrf forbidden for wrong token, got status=%d called=%v", status, called)
	}

	status, called = run(http.MethodPost, "/console/vaults", "token-1", "", Session{CSRFToken: "token-1"})
	if status != http.StatusOK || !called {
		t.Fatalf("expected csrf pass for header token, got status=%d called=%v", status, called)
	}

	status, called = run(http.MethodPost, "/console/vaults", "", "token-1", Session{CSRFToken: "token-1"})
	if status != http.StatusOK || !called {
		t.Fatalf("expected csrf pass for form token, got status=%d called=%v", status, called)
	}

	status, called = run(http.MethodGet, "/console/vaults", "", "", Session{CSRFToken: "token-1"})
	if status != http.StatusOK || !called {
		t.Fatalf("expected csrf bypass for GET, got status=%d called=%v", status, called)
	}

	status, called = run(http.MethodPost, "/obsidian/sync/changes", "", "", Session{})
	if status != http.StatusOK || !called {
		t.Fatalf("expected csrf bypass for non-console path, got status=%d called=%v", status, called)
	}
}

func TestConsoleAuthMiddleware(t *testing.T) {
	e := echo.New()
	mw := ConsoleAuthMiddleware()

	handler := mw(func(c *echo.Context) error {
		return c.NoContent(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/console/vaults", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := handler(c); err != nil {
		t.Fatalf("auth middleware: %v", err)
	}
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/console/login" {
		t.Fatalf("unexpected unauthenticated response: %d %s", rec.Code, rec.Header().Get("Location"))
	}

	req = httptest.NewRequest(http.MethodGet, "/console/vaults", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	c.Set("session.user_id", "user-1")
	if err := handler(c); err != nil {
		t.Fatalf("auth middleware: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected authenticated status: %d", rec.Code)
	}
}

func TestConsoleInitCheckMiddleware(t *testing.T) {
	e := echo.New()
	repo := setupMiddlewareRepo(t)
	mw := ConsoleInitCheckMiddleware(repo)

	run := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetPath(path)
		handler := mw(func(c *echo.Context) error {
			return c.NoContent(http.StatusOK)
		})
		if err := handler(c); err != nil {
			t.Fatalf("init check middleware: %v", err)
		}
		return rec
	}

	rec := run("/console/vaults")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/console/init" {
		t.Fatalf("unexpected no-user console response: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	rec = run("/console/init")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected console init pass before first user, got: %d", rec.Code)
	}
	rec = run("/console/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected console health bypass, got: %d", rec.Code)
	}

	tenantID := uuid.NewString()
	userID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(context.Background(), userID, tenantID, "admin", "hash"); err != nil {
		t.Fatal(err)
	}

	rec = run("/console/init")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/console/login" {
		t.Fatalf("unexpected initialized console init response: %d %s", rec.Code, rec.Header().Get("Location"))
	}
	rec = run("/console/vaults")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected console vaults pass after init, got: %d", rec.Code)
	}
	rec = run("/console/domain-check")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected domain-check bypass, got: %d", rec.Code)
	}
	rec = run("/api/ping")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected non-console bypass, got: %d", rec.Code)
	}
}

func TestConsoleNoIndexMiddleware(t *testing.T) {
	e := echo.New()
	mw := ConsoleNoIndexMiddleware()

	req := httptest.NewRequest(http.MethodGet, "/console/vaults", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := mw(func(c *echo.Context) error {
		c.Response().Header().Set("X-Robots-Tag", "index, follow")
		return c.String(http.StatusOK, "ok")
	})
	if err := handler(c); err != nil {
		t.Fatalf("noindex middleware: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("unexpected robots header: %s", got)
	}
}

func setupMiddlewareRepo(t *testing.T) *repository.Repository {
	t.Helper()
	dataPath := filepath.Join(t.TempDir(), "data")
	t.Setenv("DATA_PATH", dataPath)

	cfg, err := config.Load(middlewareRepoRoot())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	sqlDB, err := db.OpenSQLite(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return repository.New(sqlDB)
}

func middlewareRepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
