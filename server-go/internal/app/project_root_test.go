package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectProjectRootWalksUpFromNestedDir(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalWD) })

	nestedDir := filepath.Join(projectRootRepoRoot(t), "server-go", "cmd", "mdbrain")
	if err := os.Chdir(nestedDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}

	projectRoot, err := DetectProjectRoot()
	if err != nil {
		t.Fatalf("detect project root: %v", err)
	}
	if projectRoot != projectRootRepoRoot(t) {
		t.Fatalf("unexpected project root: got %s want %s", projectRoot, projectRootRepoRoot(t))
	}
}

func projectRootRepoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
