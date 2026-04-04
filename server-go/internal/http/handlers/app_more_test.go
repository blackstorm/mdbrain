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
	"mdbrain.dev/internal/templatex"
)

func setupAppHandlerWithRenderer(t *testing.T) (*AppHandler, *repository.Repository, string) {
	t.Helper()

	cfg, repo, objectStore := setupTestCore(t)
	renderer, err := templatex.New(cfg.TemplateRoot())
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Test Org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "notes.example.com", "sync-key-app"); err != nil {
		t.Fatal(err)
	}
	return NewAppHandler(cfg, repo, objectStore, renderer), repo, vaultID
}

func TestGetNoteHomeAndRootRendering(t *testing.T) {
	handler, repo, vaultID := setupAppHandlerWithRenderer(t)
	ctx := context.Background()
	tenantID := mustTenantID(t, repo, vaultID)

	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "a.md", "note-a", strPtr("# A"), strPtr("{}"), strPtr("hash-a"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "b.md", "note-b", strPtr("# B"), strPtr("{}"), strPtr("hash-b"), nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "notes.example.com"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	if err := handler.GetNote(c); err != nil {
		t.Fatalf("get home note: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected home status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "a.md") || !strings.Contains(rec.Body.String(), "b.md") {
		t.Fatalf("expected note list in home response, got body=%s", rec.Body.String())
	}

	rootNoteID := "note-a"
	if err := repo.UpdateVaultRootNote(ctx, vaultID, &rootNoteID); err != nil {
		t.Fatal(err)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "notes.example.com"
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	if err := handler.GetNote(c); err != nil {
		t.Fatalf("get root note: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected root status: %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "<h1>A</h1>") {
		t.Fatalf("expected root note content, got body=%s", rec.Body.String())
	}
}

func TestGetNotePathCorrectionAndHTMXPushURL(t *testing.T) {
	handler, repo, vaultID := setupAppHandlerWithRenderer(t)
	ctx := context.Background()
	tenantID := mustTenantID(t, repo, vaultID)

	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "root.md", "note-root", strPtr("# Root"), strPtr("{}"), strPtr("hash-root"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "a.md", "note-a", strPtr("# A"), strPtr("{}"), strPtr("hash-a"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "b.md", "note-b", strPtr("# B"), strPtr("{}"), strPtr("hash-b"), nil); err != nil {
		t.Fatal(err)
	}
	rootNoteID := "note-root"
	if err := repo.UpdateVaultRootNote(ctx, vaultID, &rootNoteID); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/note-a+missing-note", nil)
	req.Host = "notes.example.com"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/*")
	c.SetPathValues(echo.PathValues{{Name: "*", Value: "note-a+missing-note"}})
	if err := handler.GetNote(c); err != nil {
		t.Fatalf("get note with missing path id: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected corrected path status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Replace-Url"); got != "/note-a" {
		t.Fatalf("expected HX-Replace-Url /note-a, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/note-b", nil)
	req.Host = "notes.example.com"
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Current-Url", "/")
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.SetPath("/*")
	c.SetPathValues(echo.PathValues{{Name: "*", Value: "note-b"}})
	if err := handler.GetNote(c); err != nil {
		t.Fatalf("get htmx note: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected htmx status: %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("HX-Push-Url"); got != "/note-root+note-b" {
		t.Fatalf("expected HX-Push-Url /note-root+note-b, got %q", got)
	}
}

func TestGetNoteRendersBrokenPathAsNotFound(t *testing.T) {
	handler, _, _ := setupAppHandlerWithRenderer(t)

	req := httptest.NewRequest(http.MethodGet, "/missing-1+missing-2", nil)
	req.Host = "notes.example.com"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/*")
	c.SetPathValues(echo.PathValues{{Name: "*", Value: "missing-1+missing-2"}})

	if err := handler.GetNote(c); err != nil {
		t.Fatalf("get missing notes: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for fully broken note path, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestServeAssetS3ModeReturnsNotFound(t *testing.T) {
	handler, repo, vaultID := setupAppHandlerWithRenderer(t)
	ctx := context.Background()
	tenantID := mustTenantID(t, repo, vaultID)

	localStore := handler.store.(*storage.LocalStore)
	if err := localStore.PutObject(vaultID, "assets/client-1", []byte("asset-data"), "image/png"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "client-1", "img/a.png", store.AssetObjectKey("client-1", "png"), 10, "image/png", "md5-a"); err != nil {
		t.Fatal(err)
	}

	handler.cfg.StorageType = "s3"

	req := httptest.NewRequest(http.MethodGet, "/storage/assets/client-1", nil)
	req.Host = "notes.example.com"
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/storage/*")
	c.SetPathValues(echo.PathValues{{Name: "*", Value: "assets/client-1"}})
	if err := handler.ServeAsset(c); err != nil {
		t.Fatalf("serve asset in s3 mode: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for local asset serving in s3 mode, got %d", rec.Code)
	}
}
