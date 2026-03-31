package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaultsAndSecrets(t *testing.T) {
	t.Setenv("DATA_PATH", filepath.Join(t.TempDir(), "data"))
	projectRoot := filepath.Clean(filepath.Join("..", ".."))

	cfg, err := Load(context.Background(), projectRoot)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.AppPort != 8080 || cfg.ConsolePort != 9090 {
		t.Fatalf("unexpected ports: %#v", cfg)
	}
	if cfg.StorageType != "local" {
		t.Fatalf("unexpected storage type: %s", cfg.StorageType)
	}
	if cfg.SessionSecret == "" || cfg.HealthToken == "" {
		t.Fatalf("expected generated secrets")
	}
	if _, err := os.Stat(filepath.Join(cfg.DataPath, ".secrets.edn")); err != nil {
		t.Fatalf("expected secrets file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataPath, ".health-token")); err != nil {
		t.Fatalf("expected health token file: %v", err)
	}
}

func TestLoadReadsExistingSecret(t *testing.T) {
	dataPath := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dataPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataPath, ".secrets.edn"), []byte("{:session-secret \"abc123\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataPath, ".health-token"), []byte("token-1"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATA_PATH", dataPath)

	cfg, err := Load(context.Background(), filepath.Clean(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SessionSecret != "abc123" {
		t.Fatalf("expected existing secret, got %q", cfg.SessionSecret)
	}
	if cfg.HealthToken != "token-1" {
		t.Fatalf("expected existing token, got %q", cfg.HealthToken)
	}
}
