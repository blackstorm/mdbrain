package handlers

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"mdbrain.dev/internal/config"
	"mdbrain.dev/internal/infra/db"
	"mdbrain.dev/internal/infra/repository"
	"mdbrain.dev/internal/infra/storage"
	"mdbrain.dev/internal/templatex"
)

func setupTestCore(t *testing.T) (*config.Config, *repository.Repository, *storage.LocalStore) {
	t.Helper()

	dataPath := filepath.Join(t.TempDir(), "data")
	t.Setenv("DATA_PATH", dataPath)
	cfg, err := config.Load(context.Background(), handlersRepoRoot())
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	sqlDB, err := db.OpenSQLite(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.RunMigrations(context.Background(), sqlDB, cfg.MigrationDir); err != nil {
		t.Fatalf("run migration: %v", err)
	}

	repo := repository.New(sqlDB)
	objectStore, err := storage.NewLocalStore(cfg.LocalStoragePath)
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}
	return cfg, repo, objectStore
}

func setupTestCoreWithRenderer(t *testing.T) (*config.Config, *repository.Repository, *storage.LocalStore, *templatex.Renderer) {
	t.Helper()

	cfg, repo, objectStore := setupTestCore(t)
	renderer, err := templatex.New(cfg.TemplateRoot)
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}
	return cfg, repo, objectStore, renderer
}

func handlersRepoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
