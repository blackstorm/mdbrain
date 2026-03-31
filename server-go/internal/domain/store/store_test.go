package store

import "testing"

func TestNormalizePath(t *testing.T) {
	if got := NormalizePath("/a/./b/../c"); got != "a/c" {
		t.Fatalf("unexpected normalized path: %s", got)
	}
}

func TestKeys(t *testing.T) {
	if got := VaultPrefix("a-b-c"); got != "abc/" {
		t.Fatalf("unexpected vault prefix: %s", got)
	}
	if got := AssetObjectKey("asset-1", "PNG"); got != "assets/asset-1.png" {
		t.Fatalf("unexpected asset key: %s", got)
	}
	if got := FaviconObjectKey("site/logo/hash.png"); got != "site/logo/hash.favicon.png" {
		t.Fatalf("unexpected favicon key: %s", got)
	}
}
