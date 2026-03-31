package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TestConsoleAssetURL(t *testing.T) {
	got := ConsoleAssetURL("vault-123", "site/logo/abc.png")
	if got != "/console/storage/vault-123/site/logo/abc.png" {
		t.Fatalf("unexpected asset url: %s", got)
	}
}

func TestServeConsoleAsset(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleCommonHandler(repo, objectStore)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "blog.example.com", "sync"); err != nil {
		t.Fatal(err)
	}
	if err := objectStore.PutObject(vaultID, "site/logo/test.png", []byte("pngdata"), "image/png"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/console/storage/"+vaultID+"/site/logo/test.png", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/storage/:id/*")
	c.SetPathValues(echo.PathValues{
		{Name: "id", Value: vaultID},
		{Name: "*", Value: "site/logo/test.png"},
	})

	if err := handler.ServeConsoleAsset(c); err != nil {
		t.Fatalf("serve asset: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "pngdata" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("unexpected content-type: %s", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("unexpected cache-control: %s", got)
	}
}

func TestServeConsoleAssetVaultNotFound(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleCommonHandler(repo, objectStore)

	req := httptest.NewRequest(http.MethodGet, "/console/storage/not-exists/site/logo/test.png", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", uuid.NewString())
	c.SetPath("/console/storage/:id/*")
	c.SetPathValues(echo.PathValues{
		{Name: "id", Value: "not-exists"},
		{Name: "*", Value: "site/logo/test.png"},
	})

	if err := handler.ServeConsoleAsset(c); err != nil {
		t.Fatalf("serve asset: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServeConsoleAssetWrongTenant(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleCommonHandler(repo, objectStore)

	tenant1 := uuid.NewString()
	tenant2 := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenant1, "Tenant1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTenant(context.Background(), tenant2, "Tenant2"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenant1, "Blog", "blog.example.com", "sync"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/console/storage/"+vaultID+"/site/logo/test.png", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenant2)
	c.SetPath("/console/storage/:id/*")
	c.SetPathValues(echo.PathValues{
		{Name: "id", Value: vaultID},
		{Name: "*", Value: "site/logo/test.png"},
	})

	if err := handler.ServeConsoleAsset(c); err != nil {
		t.Fatalf("serve asset: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServeConsoleAssetMissingPath(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleCommonHandler(repo, objectStore)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "blog.example.com", "sync"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/console/storage/"+vaultID+"/", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/storage/:id/*")
	c.SetPathValues(echo.PathValues{
		{Name: "id", Value: vaultID},
		{Name: "*", Value: ""},
	})

	if err := handler.ServeConsoleAsset(c); err != nil {
		t.Fatalf("serve asset: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServeConsoleAssetNotFoundInStorage(t *testing.T) {
	_, repo, objectStore := setupTestCore(t)
	handler := NewConsoleCommonHandler(repo, objectStore)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Acme"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "blog.example.com", "sync"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/console/storage/"+vaultID+"/site/logo/missing.png", nil)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.Set("session.tenant_id", tenantID)
	c.SetPath("/console/storage/:id/*")
	c.SetPathValues(echo.PathValues{
		{Name: "id", Value: vaultID},
		{Name: "*", Value: "site/logo/missing.png"},
	})

	if err := handler.ServeConsoleAsset(c); err != nil {
		t.Fatalf("serve asset: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
}

