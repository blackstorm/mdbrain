package config

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Config struct {
	ProjectRoot string
	DataPath    string
	Environment string

	AppHost     string
	AppPort     int
	ConsoleHost string
	ConsolePort int

	StorageType      string
	LocalStoragePath string

	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Region    string
	S3Bucket    string
	S3PublicURL string

	SessionSecret string
	HealthToken   string

	OnDemandTLSEnabled bool
}

func Load(projectRoot string) (*Config, error) {
	dataPath := envOr("DATA_PATH", "data")
	hostFallback := envOr("HOST", "0.0.0.0")
	cfg := &Config{
		ProjectRoot:        projectRoot,
		DataPath:           dataPath,
		Environment:        envOr("ENVIRONMENT", "development"),
		AppHost:            envOr("APP_HOST", hostFallback),
		AppPort:            envIntOr("APP_PORT", 8080),
		ConsoleHost:        envOr("CONSOLE_HOST", hostFallback),
		ConsolePort:        envIntOr("CONSOLE_PORT", 9090),
		StorageType:        strings.ToLower(envOr("STORAGE_TYPE", "local")),
		LocalStoragePath:   envOr("LOCAL_STORAGE_PATH", filepath.Join(dataPath, "storage")),
		S3Endpoint:         os.Getenv("S3_ENDPOINT"),
		S3AccessKey:        os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:        os.Getenv("S3_SECRET_KEY"),
		S3Region:           envOr("S3_REGION", "us-east-1"),
		S3Bucket:           envOr("S3_BUCKET", "mdbrain"),
		S3PublicURL:        os.Getenv("S3_PUBLIC_URL"),
		OnDemandTLSEnabled: os.Getenv("CADDY_ON_DEMAND_TLS_ENABLED") == "true",
	}

	sessionSecret, err := loadOrCreateSessionSecret(cfg.DataPath)
	if err != nil {
		return nil, err
	}
	cfg.SessionSecret = firstNonEmpty(os.Getenv("SESSION_SECRET"), sessionSecret)

	healthToken, err := loadOrCreateHealthToken(cfg.DataPath)
	if err != nil {
		return nil, err
	}
	cfg.HealthToken = healthToken

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Validate() error {
	switch c.StorageType {
	case "local":
		return nil
	case "s3":
		var errs []string
		if strings.TrimSpace(c.S3Endpoint) == "" {
			errs = append(errs, "missing S3_ENDPOINT: required when STORAGE_TYPE=s3")
		}
		if strings.TrimSpace(c.S3AccessKey) == "" {
			errs = append(errs, "missing S3_ACCESS_KEY: required when STORAGE_TYPE=s3")
		}
		if strings.TrimSpace(c.S3SecretKey) == "" {
			errs = append(errs, "missing S3_SECRET_KEY: required when STORAGE_TYPE=s3")
		}
		if strings.TrimSpace(c.S3PublicURL) == "" {
			errs = append(errs, "missing S3_PUBLIC_URL: required when STORAGE_TYPE=s3")
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		return nil
	default:
		return fmt.Errorf("unknown STORAGE_TYPE: %s. supported: local, s3", c.StorageType)
	}
}

func (c *Config) Production() bool {
	return c.Environment == "production"
}

func (c *Config) TemplateRoot() string {
	return filepath.Join(c.ProjectRoot, "server", "resources")
}

func (c *Config) PublicRoot() string {
	return filepath.Join(c.ProjectRoot, "server", "resources", "publics")
}

func (c *Config) MigrationDir() string {
	return filepath.Join(c.ProjectRoot, "server-go", "ent", "migrate", "migrations")
}

func (c *Config) DatabasePath() string {
	return filepath.Join(c.DataPath, "mdbrain.db")
}

func (c *Config) SessionHashKey() []byte {
	sum := md5.Sum([]byte(c.SessionSecret))
	return sum[:]
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func envIntOr(key string, fallback int) int {
	value := os.Getenv(key)
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func loadOrCreateSessionSecret(dataPath string) (string, error) {
	path := filepath.Join(dataPath, ".secrets.edn")
	content, err := os.ReadFile(path)
	if err == nil {
		if secret := parseEDNSessionSecret(string(content)); secret != "" {
			return secret, nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	secret, err := generateRandomHex(16)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	payload := fmt.Sprintf("{:session-secret %q}\n", secret)
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

func loadOrCreateHealthToken(dataPath string) (string, error) {
	path := filepath.Join(dataPath, ".health-token")
	content, err := os.ReadFile(path)
	if err == nil {
		token := strings.TrimSpace(string(content))
		if token != "" {
			return token, nil
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	token, err := generateRandomHex(32)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(token), 0o600); err != nil {
		return "", err
	}
	return token, nil
}

var sessionSecretPattern = regexp.MustCompile(`:session-secret\s+"([^"]+)"`)

func parseEDNSessionSecret(content string) string {
	matches := sessionSecretPattern.FindStringSubmatch(content)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func generateRandomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
