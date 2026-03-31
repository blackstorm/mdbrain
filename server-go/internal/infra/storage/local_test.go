package storage

import (
	"testing"

	"mdbrain.dev/internal/domain/store"
)

func TestLocalStoreLifecycle(t *testing.T) {
	s, err := NewLocalStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if err := s.PutObject("vault-1", "assets/a.png", []byte("abc"), "image/png"); err != nil {
		t.Fatal(err)
	}

	head, err := s.HeadObject("vault-1", "assets/a.png")
	if err != nil {
		t.Fatal(err)
	}
	if head == nil || head.ContentLength != 3 {
		t.Fatalf("unexpected metadata: %#v", head)
	}

	obj, err := s.GetObject("vault-1", "assets/a.png")
	if err != nil {
		t.Fatal(err)
	}
	body, err := ReadAll(obj)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "abc" {
		t.Fatalf("unexpected body: %q", string(body))
	}

	if got := s.PublicAssetURL("vault-1", "assets/a.png"); got != "/storage/assets/a.png" {
		t.Fatalf("unexpected public URL: %s", got)
	}

	if err := s.DeleteVaultObjects("vault-1"); err != nil {
		t.Fatal(err)
	}
	obj, err = s.GetObject("vault-1", "assets/a.png")
	if err != nil {
		t.Fatal(err)
	}
	if obj != nil {
		t.Fatalf("expected deleted object, got %#v", obj)
	}

	if store.VaultPrefix("vault-1") == "" {
		t.Fatal("expected vault prefix")
	}
}
