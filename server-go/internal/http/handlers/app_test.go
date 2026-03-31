package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"mdbrain.dev/internal/domain/store"
	"mdbrain.dev/internal/infra/repository"
	"mdbrain.dev/internal/infra/storage"
)

func setupAppHandler(t *testing.T) (*AppHandler, *repository.Repository, string) {
	t.Helper()
	cfg, repo, objectStore := setupTestCore(t)
	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Test Org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "notes.example.com", "sync-key-app"); err != nil {
		t.Fatal(err)
	}
	return NewAppHandler(cfg, repo, objectStore, nil), repo, vaultID
}

func TestGetNoteRejectsInvalidHost(t *testing.T) {
	handler, _, _ := setupAppHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/any-note", nil)
	req.Host = "bad host"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/*")
	c.SetPathValues(echo.PathValues{{Name: "*", Value: "any-note"}})

	if err := handler.GetNote(c); err != nil {
		t.Fatalf("get note: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "Bad request" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestServeAssetAndFavicon(t *testing.T) {
	handler, repo, vaultID := setupAppHandler(t)
	localStore := handler.store.(*storage.LocalStore)
	if err := localStore.PutObject(vaultID, "assets/client-1", []byte("asset-data"), "image/png"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/storage/assets/client-1", nil)
	req.Host = "notes.example.com"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/storage/*")
	c.SetPathValues(echo.PathValues{{Name: "*", Value: "assets/client-1"}})
	if err := handler.ServeAsset(c); err != nil {
		t.Fatalf("serve asset: %v", err)
	}
	if rec.Code != http.StatusOK || rec.Body.String() != "asset-data" {
		t.Fatalf("unexpected asset response: %d %s", rec.Code, rec.Body.String())
	}

	logoKey := "site/logo/hash123.png"
	if err := repo.UpdateVaultLogo(context.Background(), vaultID, &logoKey); err != nil {
		t.Fatal(err)
	}
	if err := localStore.PutObject(vaultID, store.FaviconObjectKey(logoKey), []byte("favicon"), "image/png"); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/favicon.ico?v=1", nil)
	req.Host = "notes.example.com"
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	if err := handler.ServeFavicon(c); err != nil {
		t.Fatalf("serve favicon: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("unexpected favicon status: %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), ".favicon.") {
		t.Fatalf("unexpected favicon location: %s", rec.Header().Get("Location"))
	}
}
