package config

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateAllowsLocalStorage(t *testing.T) {
	cfg := &Config{StorageType: "local"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate local storage: %v", err)
	}
}

func TestValidateRejectsUnknownStorageType(t *testing.T) {
	cfg := &Config{StorageType: "ftp"}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "unknown STORAGE_TYPE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateS3RequiresAllFields(t *testing.T) {
	cfg := &Config{StorageType: "s3"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	for _, part := range []string{"S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_PUBLIC_URL"} {
		if !strings.Contains(err.Error(), part) {
			t.Fatalf("expected %s in error: %v", part, err)
		}
	}
}

func TestValidateS3SucceedsWithRequiredFields(t *testing.T) {
	cfg := &Config{
		StorageType: "s3",
		S3Endpoint:  "https://s3.example.com",
		S3AccessKey: "access",
		S3SecretKey: "secret",
		S3PublicURL: "https://cdn.example.com",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate s3: %v", err)
	}
}

func TestProductionTrueForProduction(t *testing.T) {
	if !(&Config{Environment: "production"}).Production() {
		t.Fatal("expected production=true")
	}
}

func TestProductionFalseForDevelopment(t *testing.T) {
	if (&Config{Environment: "development"}).Production() {
		t.Fatal("expected production=false")
	}
}

func TestSessionHashKeyDeterministic(t *testing.T) {
	first := (&Config{SessionSecret: "secret-1"}).SessionHashKey()
	second := (&Config{SessionSecret: "secret-1"}).SessionHashKey()
	if string(first) != string(second) {
		t.Fatal("expected deterministic session hash key")
	}
}

func TestParseEDNSessionSecretInvalidInput(t *testing.T) {
	if got := parseEDNSessionSecret("{:other-key \"abc\"}"); got != "" {
		t.Fatalf("unexpected secret: %q", got)
	}
}

func TestFirstNonEmptySkipsWhitespace(t *testing.T) {
	if got := firstNonEmpty(" ", "\n", "value"); got != "value" {
		t.Fatalf("unexpected first non-empty value: %q", got)
	}
}

func TestEnvOrFallsBackOnWhitespace(t *testing.T) {
	t.Setenv("MDBRAIN_TEST_ENV_OR", "   ")
	if got := envOr("MDBRAIN_TEST_ENV_OR", "fallback"); got != "fallback" {
		t.Fatalf("unexpected envOr value: %q", got)
	}
}

func TestEnvOrUsesValue(t *testing.T) {
	t.Setenv("MDBRAIN_TEST_ENV_OR", "value")
	if got := envOr("MDBRAIN_TEST_ENV_OR", "fallback"); got != "value" {
		t.Fatalf("unexpected envOr value: %q", got)
	}
}

func TestEnvIntOrFallsBackOnInvalid(t *testing.T) {
	t.Setenv("MDBRAIN_TEST_ENV_INT", "abc")
	if got := envIntOr("MDBRAIN_TEST_ENV_INT", 42); got != 42 {
		t.Fatalf("unexpected envIntOr value: %d", got)
	}
}

func TestEnvIntOrUsesParsedValue(t *testing.T) {
	t.Setenv("MDBRAIN_TEST_ENV_INT", "1234")
	if got := envIntOr("MDBRAIN_TEST_ENV_INT", 42); got != 1234 {
		t.Fatalf("unexpected envIntOr value: %d", got)
	}
}

func TestLoadRespectsSessionSecretEnvOverride(t *testing.T) {
	t.Setenv("DATA_PATH", filepath.Join(t.TempDir(), "data"))
	t.Setenv("SESSION_SECRET", "from-env")

	cfg, err := Load(context.Background(), filepath.Clean(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.SessionSecret != "from-env" {
		t.Fatalf("unexpected session secret: %q", cfg.SessionSecret)
	}
}

func TestLoadAppliesRuntimeOverrides(t *testing.T) {
	t.Setenv("DATA_PATH", filepath.Join(t.TempDir(), "data"))
	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("HOST", "127.0.0.1")
	t.Setenv("APP_PORT", "18080")
	t.Setenv("CONSOLE_PORT", "19090")
	t.Setenv("STORAGE_TYPE", "local")
	t.Setenv("CADDY_ON_DEMAND_TLS_ENABLED", "true")

	cfg, err := Load(context.Background(), filepath.Clean(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Environment != "production" || cfg.AppHost != "127.0.0.1" || cfg.ConsoleHost != "127.0.0.1" {
		t.Fatalf("unexpected host/environment config: %#v", cfg)
	}
	if cfg.AppPort != 18080 || cfg.ConsolePort != 19090 {
		t.Fatalf("unexpected port config: %#v", cfg)
	}
	if !cfg.OnDemandTLSEnabled {
		t.Fatalf("expected on-demand TLS to be enabled")
	}
}

func TestLoadRejectsInvalidS3Configuration(t *testing.T) {
	t.Setenv("DATA_PATH", filepath.Join(t.TempDir(), "data"))
	t.Setenv("STORAGE_TYPE", "s3")

	_, err := Load(context.Background(), filepath.Clean(filepath.Join("..", "..")))
	if err == nil || !strings.Contains(err.Error(), "missing S3_ENDPOINT") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadRespectsLocalStoragePathOverride(t *testing.T) {
	t.Setenv("DATA_PATH", filepath.Join(t.TempDir(), "data"))
	t.Setenv("LOCAL_STORAGE_PATH", "/tmp/mdbrain-storage")

	cfg, err := Load(context.Background(), filepath.Clean(filepath.Join("..", "..")))
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.LocalStoragePath != "/tmp/mdbrain-storage" {
		t.Fatalf("unexpected local storage path: %s", cfg.LocalStoragePath)
	}
}

func TestParseEDNSessionSecretExtractsValue(t *testing.T) {
	if got := parseEDNSessionSecret("{:session-secret \"abc123\"}\n"); got != "abc123" {
		t.Fatalf("unexpected parsed secret: %q", got)
	}
}

func TestFirstNonEmptyReturnsEmptyWhenAllBlank(t *testing.T) {
	if got := firstNonEmpty("", " ", "\n"); got != "" {
		t.Fatalf("unexpected first non-empty value: %q", got)
	}
}

func TestGenerateRandomHexLength(t *testing.T) {
	got, err := generateRandomHex(8)
	if err != nil {
		t.Fatalf("generate random hex: %v", err)
	}
	if len(got) != 16 {
		t.Fatalf("unexpected random hex length: %d", len(got))
	}
}

func TestGenerateRandomHexProducesDifferentValues(t *testing.T) {
	first, err := generateRandomHex(8)
	if err != nil {
		t.Fatalf("generate first random hex: %v", err)
	}
	second, err := generateRandomHex(8)
	if err != nil {
		t.Fatalf("generate second random hex: %v", err)
	}
	if first == second {
		t.Fatalf("expected distinct random hex values, got %q", first)
	}
}
