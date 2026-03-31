package store

import "testing"

func TestVaultPrefixRemovesAllHyphens(t *testing.T) {
	if got := VaultPrefix("vault-a-b-c"); got != "vaultabc/" {
		t.Fatalf("unexpected vault prefix: %s", got)
	}
}

func TestNormalizePathEmpty(t *testing.T) {
	if got := NormalizePath(""); got != "" {
		t.Fatalf("unexpected normalized path: %q", got)
	}
}

func TestNormalizePathWindowsSeparators(t *testing.T) {
	if got := NormalizePath(`folder\child\file.md`); got != "folder/child/file.md" {
		t.Fatalf("unexpected normalized path: %s", got)
	}
}

func TestNormalizePathCollapsesTraversal(t *testing.T) {
	if got := NormalizePath("a/b/../../c/d"); got != "c/d" {
		t.Fatalf("unexpected normalized path: %s", got)
	}
}

func TestNormalizePathDoesNotGoAboveRoot(t *testing.T) {
	if got := NormalizePath("../../safe/file.md"); got != "safe/file.md" {
		t.Fatalf("unexpected normalized path: %s", got)
	}
}

func TestExtensionFromPathLowercasesAndSanitizes(t *testing.T) {
	if got := ExtensionFromPath("image.PnG"); got != "png" {
		t.Fatalf("unexpected extension: %s", got)
	}
}

func TestExtensionFromPathKeepsAllowedSymbols(t *testing.T) {
	if got := ExtensionFromPath("archive.a+b_c-1"); got != "a+b_c-1" {
		t.Fatalf("unexpected extension: %s", got)
	}
}

func TestExtensionFromPathReturnsEmptyWhenMissing(t *testing.T) {
	if got := ExtensionFromPath("README"); got != "" {
		t.Fatalf("unexpected extension: %s", got)
	}
}

func TestExtensionFromPathUsesLastExtension(t *testing.T) {
	if got := ExtensionFromPath("archive.tar.gz"); got != "gz" {
		t.Fatalf("unexpected extension: %s", got)
	}
}

func TestAssetObjectKeyWithoutExtension(t *testing.T) {
	if got := AssetObjectKey("asset-1", ""); got != "assets/asset-1" {
		t.Fatalf("unexpected asset object key: %s", got)
	}
}

func TestAssetObjectKeyNormalizesExtension(t *testing.T) {
	if got := AssetObjectKey("asset-1", "JpEg"); got != "assets/asset-1.jpeg" {
		t.Fatalf("unexpected asset object key: %s", got)
	}
}

func TestLogoObjectKeyUsesNormalizedExtension(t *testing.T) {
	if got := LogoObjectKey("hash123", "PNG"); got != "site/logo/hash123.png" {
		t.Fatalf("unexpected logo object key: %s", got)
	}
}

func TestFaviconObjectKeyAddsFaviconSuffix(t *testing.T) {
	if got := FaviconObjectKey("site/logo/hash123.png"); got != "site/logo/hash123.favicon.png" {
		t.Fatalf("unexpected favicon object key: %s", got)
	}
}

func TestFaviconObjectKeyReturnsEmptyWithoutExtension(t *testing.T) {
	if got := FaviconObjectKey("site/logo/hash123"); got != "" {
		t.Fatalf("unexpected favicon object key: %s", got)
	}
}

func TestNormalizePathSkipsCurrentDirectorySegments(t *testing.T) {
	if got := NormalizePath("./notes/./daily.md"); got != "notes/daily.md" {
		t.Fatalf("unexpected normalized path: %s", got)
	}
}

func TestExtensionFromPathTruncatesLongExtensions(t *testing.T) {
	if got := ExtensionFromPath("archive." + "abcdefghijklmnopqrstuvwxyz1234567890"); len(got) != 32 {
		t.Fatalf("expected extension length 32, got %d (%q)", len(got), got)
	}
}

func TestAssetObjectKeySanitizesWeirdExtension(t *testing.T) {
	if got := AssetObjectKey("asset-1", "P$N&G"); got != "assets/asset-1.png" {
		t.Fatalf("unexpected asset object key: %s", got)
	}
}

func TestLogoObjectKeyWithEmptyExtensionKeepsDot(t *testing.T) {
	if got := LogoObjectKey("hash123", ""); got != "site/logo/hash123." {
		t.Fatalf("unexpected logo object key: %s", got)
	}
}

func TestFaviconObjectKeyUsesLastExtension(t *testing.T) {
	if got := FaviconObjectKey("site/logo/hash123.tar.gz"); got != "site/logo/hash123.tar.favicon.gz" {
		t.Fatalf("unexpected favicon object key: %s", got)
	}
}
