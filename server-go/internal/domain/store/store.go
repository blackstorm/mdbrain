package store

import (
	"io"
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

func NormalizePath(path string) string {
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	stack := make([]string, 0, len(parts))
	for _, part := range parts {
		switch {
		case part == "", part == ".":
			continue
		case part == "..":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		default:
			stack = append(stack, part)
		}
	}
	return strings.Join(stack, "/")
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
	base := strings.TrimSuffix(logoObjectKey, "."+ext)
	return base + ".favicon." + ext
}
