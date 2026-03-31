package storage

import (
	"os"
	"path/filepath"
	"testing"

	"mdbrain.dev/internal/domain/store"
)

func TestLocalStoreOverwriteAndNestedDirectories(t *testing.T) {
	s, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}

	const (
		vaultID   = "vault-overwrite"
		objectKey = "assets/deep/nested/file.txt"
	)
	if err := s.PutObject(vaultID, objectKey, []byte("first"), "text/plain"); err != nil {
		t.Fatalf("put first object: %v", err)
	}
	if err := s.PutObject(vaultID, objectKey, []byte("second"), "text/plain"); err != nil {
		t.Fatalf("put second object: %v", err)
	}

	obj, err := s.GetObject(vaultID, objectKey)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	body, err := ReadAll(obj)
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if got := string(body); got != "second" {
		t.Fatalf("expected overwritten content, got %q", got)
	}

	fullPath := filepath.Join(s.basePath, store.VaultPrefix(vaultID), objectKey)
	if _, err := os.Stat(filepath.Dir(fullPath)); err != nil {
		t.Fatalf("expected nested directory to exist: %v", err)
	}
}

func TestLocalStoreContentTypeDetection(t *testing.T) {
	s, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}

	if err := s.PutObject("vault-ct", "assets/logo.png", []byte("png-bytes"), "application/custom"); err != nil {
		t.Fatalf("put object: %v", err)
	}

	head, err := s.HeadObject("vault-ct", "assets/logo.png")
	if err != nil {
		t.Fatalf("head object: %v", err)
	}
	if head == nil {
		t.Fatal("expected metadata, got nil")
	}
	if head.ContentType != "image/png" {
		t.Fatalf("expected detected content type image/png, got %q", head.ContentType)
	}
}

func TestLocalStoreMissingObjectHandling(t *testing.T) {
	s, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}

	obj, err := s.GetObject("vault-missing", "assets/missing.bin")
	if err != nil {
		t.Fatalf("get missing object should not fail: %v", err)
	}
	if obj != nil {
		t.Fatalf("expected nil missing object, got %#v", obj)
	}

	head, err := s.HeadObject("vault-missing", "assets/missing.bin")
	if err != nil {
		t.Fatalf("head missing object should not fail: %v", err)
	}
	if head != nil {
		t.Fatalf("expected nil missing metadata, got %#v", head)
	}

	if err := s.DeleteObject("vault-missing", "assets/missing.bin"); err != nil {
		t.Fatalf("delete missing object should not fail: %v", err)
	}
}

func TestLocalStoreDeleteVaultObjectsIsolation(t *testing.T) {
	s, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}

	if err := s.PutObject("vault-a", "assets/a.txt", []byte("a"), "text/plain"); err != nil {
		t.Fatalf("put object vault-a: %v", err)
	}
	if err := s.PutObject("vault-b", "assets/b.txt", []byte("b"), "text/plain"); err != nil {
		t.Fatalf("put object vault-b: %v", err)
	}

	if err := s.DeleteVaultObjects("vault-a"); err != nil {
		t.Fatalf("delete vault-a objects: %v", err)
	}

	objA, err := s.GetObject("vault-a", "assets/a.txt")
	if err != nil {
		t.Fatalf("get vault-a object: %v", err)
	}
	if objA != nil {
		t.Fatalf("expected vault-a object deleted, got %#v", objA)
	}

	objB, err := s.GetObject("vault-b", "assets/b.txt")
	if err != nil {
		t.Fatalf("get vault-b object: %v", err)
	}
	bodyB, err := ReadAll(objB)
	if err != nil {
		t.Fatalf("read vault-b object: %v", err)
	}
	if got := string(bodyB); got != "b" {
		t.Fatalf("expected vault-b object unchanged, got %q", got)
	}
}

func TestLocalStoreFullCRUDCycle(t *testing.T) {
	s, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatalf("new local store: %v", err)
	}

	const (
		vaultID   = "vault-crud"
		objectKey = "assets/doc.pdf"
	)
	content := []byte("pdf-data")
	if err := s.PutObject(vaultID, objectKey, content, "application/pdf"); err != nil {
		t.Fatalf("put object: %v", err)
	}

	head, err := s.HeadObject(vaultID, objectKey)
	if err != nil {
		t.Fatalf("head object: %v", err)
	}
	if head == nil || head.ContentLength != int64(len(content)) {
		t.Fatalf("unexpected metadata: %#v", head)
	}

	obj, err := s.GetObject(vaultID, objectKey)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	read, err := ReadAll(obj)
	if err != nil {
		t.Fatalf("read object: %v", err)
	}
	if string(read) != string(content) {
		t.Fatalf("unexpected object content: %q", string(read))
	}

	if err := s.DeleteObject(vaultID, objectKey); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	obj, err = s.GetObject(vaultID, objectKey)
	if err != nil {
		t.Fatalf("get deleted object: %v", err)
	}
	if obj != nil {
		t.Fatalf("expected deleted object to be nil, got %#v", obj)
	}
}

func TestReadAllNilObjectAndNilBody(t *testing.T) {
	body, err := ReadAll(nil)
	if err != nil {
		t.Fatalf("read nil object: %v", err)
	}
	if body != nil {
		t.Fatalf("expected nil body for nil object, got %#v", body)
	}

	body, err = ReadAll(&store.Object{})
	if err != nil {
		t.Fatalf("read object with nil body: %v", err)
	}
	if body != nil {
		t.Fatalf("expected nil body for nil object body, got %#v", body)
	}
}
