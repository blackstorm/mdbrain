package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mdbrain.dev/internal/config"
	dbinfra "mdbrain.dev/internal/infra/db"
)

func TestBuildWiresAppAndConsoleRoutes(t *testing.T) {
	t.Setenv("DATA_PATH", filepath.Join(t.TempDir(), "data"))
	t.Setenv("APP_PORT", "18080")
	t.Setenv("CONSOLE_PORT", "19090")

	cfg, err := config.Load(context.Background(), appRepoRoot(t))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	db, err := dbinfra.OpenSQLite(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := dbinfra.RunMigrations(context.Background(), db, cfg.MigrationDir); err != nil {
		_ = db.Close()
		t.Fatalf("run migrations: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite after migration: %v", err)
	}

	servers, err := Build(context.Background(), appRepoRoot(t))
	if err != nil {
		t.Fatalf("build server set: %v", err)
	}
	t.Cleanup(func() {
		_ = servers.Shutdown(context.Background())
	})

	if servers.AppServer.Addr != "0.0.0.0:18080" {
		t.Fatalf("unexpected app addr: %s", servers.AppServer.Addr)
	}
	if servers.ConsoleServer.Addr != "0.0.0.0:19090" {
		t.Fatalf("unexpected console addr: %s", servers.ConsoleServer.Addr)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	servers.ConsoleServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Disallow: /") {
		t.Fatalf("unexpected robots response: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("expected noindex header on console route, got %s", got)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/console/health?token="+servers.Config.HealthToken, nil)
	servers.ConsoleServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("unexpected health response: status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/console/init", nil)
	servers.ConsoleServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected init page status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Welcome to Mdbrain") {
		t.Fatalf("expected init page content, got %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "missing.example.com"
	servers.AppServer.Handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected unknown app host forbidden, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func appRepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
