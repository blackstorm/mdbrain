package repository

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"

	"mdbrain.dev/internal/config"
	dbinfra "mdbrain.dev/internal/infra/db"
)

func setupRepository(t *testing.T) *Repository {
	t.Helper()

	dataPath := filepath.Join(t.TempDir(), "data")
	t.Setenv("DATA_PATH", dataPath)
	cfg, err := config.Load(context.Background(), repoRoot())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	db, err := dbinfra.OpenSQLite(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := dbinfra.RunMigrations(context.Background(), db, cfg.MigrationDir); err != nil {
		t.Fatalf("migrate db: %v", err)
	}
	return New(db)
}

func TestUserAndVaultLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := setupRepository(t)

	tenantID := uuid.NewString()
	userID := uuid.NewString()
	vaultID := uuid.NewString()

	if err := repo.CreateTenant(ctx, tenantID, "Test Org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateUser(ctx, userID, tenantID, "admin", "hash"); err != nil {
		t.Fatal(err)
	}
	hasUser, err := repo.HasAnyUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !hasUser {
		t.Fatal("expected at least one user")
	}
	if err := repo.CreateVault(ctx, vaultID, tenantID, "Blog", "blog.example.com", "sync-1"); err != nil {
		t.Fatal(err)
	}

	vault, err := repo.GetVaultByDomain(ctx, "blog.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if vault.ID != vaultID || vault.SyncKey != "sync-1" {
		t.Fatalf("unexpected vault: %#v", vault)
	}
}

func TestUpsertNoteAssetAndRefs(t *testing.T) {
	ctx := context.Background()
	repo := setupRepository(t)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(ctx, tenantID, "Test Org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(ctx, vaultID, tenantID, "Blog", "blog.example.com", "sync-1"); err != nil {
		t.Fatal(err)
	}

	noteID := "note-1"
	hash := "hash-1"
	now := time.Now().UTC()
	if err := repo.UpsertNote(ctx, uuid.NewString(), tenantID, vaultID, "a.md", noteID, strPtr("hello"), strPtr("{}"), &hash, &now); err != nil {
		t.Fatal(err)
	}
	note, err := repo.GetNoteByClientID(ctx, vaultID, noteID)
	if err != nil {
		t.Fatal(err)
	}
	if note.Path != "a.md" {
		t.Fatalf("unexpected note: %#v", note)
	}

	if err := repo.UpsertAsset(ctx, uuid.NewString(), tenantID, vaultID, "asset-1", "img/a.png", "assets/a.png", 10, "image/png", "md5-a"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateNoteAssetRefs(ctx, vaultID, noteID, []string{"asset-1"}); err != nil {
		t.Fatal(err)
	}
	refs, err := repo.GetAssetRefsByNote(ctx, vaultID, noteID)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0].AssetClientID != "asset-1" {
		t.Fatalf("unexpected refs: %#v", refs)
	}
}

func TestPublishStatus(t *testing.T) {
	ctx := context.Background()
	repo := setupRepository(t)

	tenantID := uuid.NewString()
	vaultID := uuid.NewString()
	if err := repo.CreateTenant(ctx, tenantID, "Test Org"); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateVault(ctx, vaultID, tenantID, "Blog", "blog.example.com", "sync-1"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RecordVaultPublishError(ctx, vaultID, "bad_request", "broken"); err != nil {
		t.Fatal(err)
	}
	vault, err := repo.GetVaultByID(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.LastPublishStatus != "error" {
		t.Fatalf("unexpected status: %#v", vault)
	}
	if err := repo.RecordVaultPublishSuccess(ctx, vaultID); err != nil {
		t.Fatal(err)
	}
	vault, err = repo.GetVaultByID(ctx, vaultID)
	if err != nil {
		t.Fatal(err)
	}
	if vault.LastPublishStatus != "ok" {
		t.Fatalf("unexpected status after success: %#v", vault)
	}
}

func strPtr(v string) *string { return &v }

func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
