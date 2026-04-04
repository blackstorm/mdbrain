package storage

import (
	"errors"
	"io"
	"mime"
	"os"
	"path/filepath"

	"mdbrain.dev/internal/config"
	"mdbrain.dev/internal/domain/store"
)

type LocalStore struct {
	basePath string
}

func NewObjectStore(cfg *config.Config) (store.ObjectStore, error) {
	switch cfg.StorageType {
	case "local":
		return NewLocalStore(cfg.LocalStoragePath)
	case "s3":
		return NewS3Store(cfg)
	default:
		return nil, errors.New("unsupported storage type")
	}
}

func NewLocalStore(basePath string) (*LocalStore, error) {
	if err := os.MkdirAll(basePath, 0o755); err != nil {
		return nil, err
	}
	return &LocalStore{basePath: basePath}, nil
}

func (s *LocalStore) PutObject(vaultID, objectKey string, content []byte, _ string) error {
	path := s.fullPath(vaultID, objectKey)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func (s *LocalStore) GetObject(vaultID, objectKey string) (*store.Object, error) {
	path := s.fullPath(vaultID, objectKey)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	modTime := stat.ModTime()
	return &store.Object{
		Body:          file,
		ContentLength: stat.Size(),
		ContentType:   guessContentType(path),
		LastModified:  &modTime,
	}, nil
}

func (s *LocalStore) DeleteObject(vaultID, objectKey string) error {
	path := s.fullPath(vaultID, objectKey)
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *LocalStore) HeadObject(vaultID, objectKey string) (*store.Metadata, error) {
	path := s.fullPath(vaultID, objectKey)
	stat, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	modTime := stat.ModTime()
	return &store.Metadata{
		ContentLength: stat.Size(),
		ContentType:   guessContentType(path),
		LastModified:  &modTime,
	}, nil
}

func (s *LocalStore) DeleteVaultObjects(vaultID string) error {
	path := filepath.Join(s.basePath, store.VaultPrefix(vaultID))
	err := os.RemoveAll(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *LocalStore) PublicAssetURL(_ string, objectKey string) string {
	return "/storage/" + objectKey
}

func (s *LocalStore) fullPath(vaultID, objectKey string) string {
	return filepath.Join(s.basePath, store.VaultPrefix(vaultID), objectKey)
}

func guessContentType(path string) string {
	if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
		return contentType
	}
	return "application/octet-stream"
}

func ReadAll(obj *store.Object) ([]byte, error) {
	if obj == nil || obj.Body == nil {
		return nil, nil
	}
	defer obj.Body.Close()
	return io.ReadAll(obj.Body)
}
