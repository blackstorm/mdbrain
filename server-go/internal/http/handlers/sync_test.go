package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	"mdbrain.dev/internal/infra/storage"
)

func setupSyncHandler(t *testing.T) (*SyncHandler, *repository.Repository, string) {
	t.Helper()

	dataPath := filepath.Join(t.TempDir(), "data")
	t.Setenv("DATA_PATH", dataPath)
	projectRoot := repoRoot()
	cfg, err := config.Load(projectRoot)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	sqlDB, err := db.OpenSQLite(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.RunMigrations(context.Background(), sqlDB, cfg.MigrationDir()); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	repo := repository.New(sqlDB)
	objectStore, err := storage.NewLocalStore(cfg.LocalStoragePath)
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(context.Background(), tenantID, "Test Org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(context.Background(), vaultID, tenantID, "Blog", "sync.example.com", "sync-key-1"); err != nil {
		t.Fatal(err)
	}
	return NewSyncHandler(repo, objectStore), repo, vaultID
}

func TestSyncChanges(t *testing.T) {
	handler, repo, vaultID := setupSyncHandler(t)
	ctx := context.Background()

	if err := repo.UpsertNote(ctx, uuid.NewString(), mustTenantID(t, repo, vaultID), vaultID, "a.md", "note-1", strPtr("A"), strPtr("{}"), strPtr("hash-a"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), mustTenantID(t, repo, vaultID), vaultID, "b.md", "note-2", strPtr("B"), strPtr("{}"), strPtr("hash-b"), nil); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertAsset(ctx, uuid.NewString(), mustTenantID(t, repo, vaultID), vaultID, "asset-1", "img/a.png", "assets/a.png", 10, "image/png", "md5-a"); err != nil {
		t.Fatal(err)
	}

	body := `{"notes":[{"id":"note-1","hash":"hash-a"},{"id":"note-2","hash":"hash-b-new"}],"assets":[{"id":"asset-1","hash":"md5-a"},{"id":"asset-2","hash":"md5-b"}]}`
	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/changes", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)

	if err := handler.SyncChanges(c); err != nil {
		t.Fatalf("sync changes: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d", rec.Code)
	}

	var payload map[string]map[string][]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload["need_upsert"]["notes"]) != 1 || payload["need_upsert"]["notes"][0]["id"] != "note-2" {
		t.Fatalf("unexpected notes upsert payload: %#v", payload)
	}
}

func TestSyncNoteAndAsset(t *testing.T) {
	handler, repo, vaultID := setupSyncHandler(t)
	ctx := context.Background()
	tenantID := mustTenantID(t, repo, vaultID)

	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-1", "img/a.png", "assets/a.png", 10, "image/png", "md5-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "Note B.md", "note-b", strPtr("B"), strPtr("{}"), strPtr("hash-b"), nil); err != nil {
		t.Fatal(err)
	}

	noteBody := `{"path":"Note A.md","content":"Links [[Note B]]","hash":"hash-a","assets":[{"id":"asset-1","hash":"md5-a"}],"linked_notes":[{"id":"note-b","hash":"hash-b"}]}`
	req := httptest.NewRequest(http.MethodPost, "/obsidian/sync/notes/note-a", strings.NewReader(noteBody))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/notes/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "note-a"}})
	if err := handler.SyncNote(c); err != nil {
		t.Fatalf("sync note: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected note status: %d body=%s", rec.Code, rec.Body.String())
	}
	refs, err := repo.GetAssetRefsByNote(ctx, vaultID, "note-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("unexpected refs: %#v", refs)
	}

	content := base64.StdEncoding.EncodeToString([]byte("pngdata"))
	assetBody := `{"path":"img/c.png","contentType":"image/png","hash":"md5-c","content":"` + content + `"}`
	req = httptest.NewRequest(http.MethodPost, "/obsidian/sync/assets/asset-c", strings.NewReader(assetBody))
	req.Header.Set("Authorization", "Bearer sync-key-1")
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec = httptest.NewRecorder()
	c = echo.New().NewContext(req, rec)
	c.SetPath("/obsidian/sync/assets/:id")
	c.SetPathValues(echo.PathValues{{Name: "id", Value: "asset-c"}})
	if err := handler.SyncAsset(c); err != nil {
		t.Fatalf("sync asset: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected asset status: %d body=%s", rec.Code, rec.Body.String())
	}
}

func mustTenantID(t *testing.T, repo *repository.Repository, vaultID string) string {
	t.Helper()
	vault, err := repo.GetVaultByID(context.Background(), vaultID)
	if err != nil {
		t.Fatal(err)
	}
	return vault.TenantID
}

func strPtr(v string) *string { return &v }

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
