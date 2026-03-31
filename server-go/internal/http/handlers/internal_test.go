package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestInternalHealthAcceptsHeaderAndQueryToken(t *testing.T) {
	cfg, repo, _ := setupTestCore(t)
	handler := NewInternalHandler(cfg, repo)
	e := echo.New()

	run := func(target string, headerToken string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if headerToken != "" {
			req.Header.Set("X-Health-Token", headerToken)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		if err := handler.Health(c); err != nil {
			t.Fatalf("health handler: %v", err)
		}
		return rec
	}

	rec := run("/console/health", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized without token, got %d", rec.Code)
	}

	rec = run("/console/health?token="+cfg.HealthToken, "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("expected query token success, got status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = run("/console/health", cfg.HealthToken)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("expected header token success, got status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestInternalRobotsReturnsDisallowAll(t *testing.T) {
	cfg, repo, _ := setupTestCore(t)
	handler := NewInternalHandler(cfg, repo)

	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.Robots(c); err != nil {
		t.Fatalf("robots handler: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("unexpected content-type: %s", got)
	}
	if body := rec.Body.String(); body != "User-agent: *\nDisallow: /\n" {
		t.Fatalf("unexpected robots body: %q", body)
	}
}

func TestInternalDomainCheckMatchesExistingVault(t *testing.T) {
	cfg, repo, _ := setupTestCore(t)
	handler := NewInternalHandler(cfg, repo)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Docs", "docs.example.com", "sync-key"); err != nil {
		t.Fatal(err)
	}

	e := echo.New()

	req := httptest.NewRequest(http.MethodGet, "/console/domain-check?domain=docs.example.com", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	if err := handler.DomainCheck(c); err != nil {
		t.Fatalf("domain check: %v", err)
	}
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("expected existing domain ok, got status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/console/domain-check?domain=missing.example.com", nil)
	rec = httptest.NewRecorder()
	c = e.NewContext(req, rec)
	if err := handler.DomainCheck(c); err != nil {
		t.Fatalf("domain check: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected missing domain 404, got %d", rec.Code)
	}
}
