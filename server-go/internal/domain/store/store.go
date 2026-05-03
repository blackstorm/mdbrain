package store

import (
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"
)

type Object struct {
	Body          io.ReadCloser
	ContentLength int64
	ContentType   string
	LastModified  *time.Time
}

type Metadata struct {
	ContentLength int64
	ContentType   string
	LastModified  *time.Time
}

type ObjectStore interface {
	PutObject(vaultID, objectKey string, content []byte, contentType string) error
	GetObject(vaultID, objectKey string) (*Object, error)
	DeleteObject(vaultID, objectKey string) error
	HeadObject(vaultID, objectKey string) (*Metadata, error)
	DeleteVaultObjects(vaultID string) error
	PublicAssetURL(vaultID, objectKey string) string
}

func VaultPrefix(vaultID string) string {
	return strings.ReplaceAll(vaultID, "-", "") + "/"
}

func NormalizePath(value string) string {
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Clean(strings.TrimLeft(value, "/"))
	for value == ".." || strings.HasPrefix(value, "../") {
		value = strings.TrimPrefix(value, "..")
		value = strings.TrimPrefix(value, "/")
	}
	if value == "." {
		return ""
	}
	return value
}

func ExtensionFromPath(path string) string {
	base := filepath.Base(path)
	ext := strings.TrimPrefix(filepath.Ext(base), ".")
	if ext == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range strings.ToLower(ext) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '+' {
			b.WriteRune(r)
		}
		if b.Len() >= 32 {
			break
		}
	}
	return b.String()
}

func AssetObjectKey(clientID string, extension string) string {
	ext := ExtensionFromPath("." + extension)
	if ext == "" {
		return "assets/" + clientID
	}
	return "assets/" + clientID + "." + ext
}

func LogoObjectKey(contentHash, extension string) string {
	return "site/logo/" + contentHash + "." + ExtensionFromPath("."+extension)
}

func FaviconObjectKey(logoObjectKey string) string {
	ext := ExtensionFromPath(logoObjectKey)
	if ext == "" {
		return ""
	}
	base := strings.TrimSuffix(logoObjectKey, path.Ext(logoObjectKey))
	return base + ".favicon." + ext
}
