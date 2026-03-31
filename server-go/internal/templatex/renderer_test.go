package templatex

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func TestRenderSupportsExtendsSafeAndDefault(t *testing.T) {
	root := filepath.Join(repoRoot(t), "server", "resources")
	r, err := New(root)
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	out, err := r.Render("templates/app/note-page.html", map[string]any{
		"vault": map[string]any{
			"name": "Docs",
		},
		"notes": []any{
			map[string]any{
				"note": map[string]any{
					"client-id":    "note-1",
					"title":        "Hello",
					"html-content": "<strong>raw-html</strong>",
					"updated-at":   "2026-03-31",
				},
				"backlinks": []any{
					map[string]any{
						"client-id": "note-2",
						"title":     "Backlink",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	if !strings.Contains(out, "<title>Hello - Docs</title>") {
		t.Fatalf("expected extends block output in title, got: %s", out)
	}
	if !strings.Contains(out, "<strong>raw-html</strong>") {
		t.Fatalf("expected safe filter to keep raw html")
	}
	if !strings.Contains(out, `<p class="backlink-desc"></p>`) {
		t.Fatalf("expected default filter to render fallback empty string")
	}
}

func TestRenderSupportsIncludeAndWith(t *testing.T) {
	root := filepath.Join(repoRoot(t), "server", "resources")
	r, err := New(root)
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	out, err := r.Render("templates/console/vault-list.html", map[string]any{
		"vaults": []any{},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	if !strings.Contains(out, "No sites yet") {
		t.Fatalf("expected include/with branch output")
	}
	if !strings.Contains(out, "<svg") {
		t.Fatalf("expected included template to render lucide icon svg")
	}
}

func TestLucideHelperInjectsAttrs(t *testing.T) {
	root := filepath.Join(repoRoot(t), "server", "resources")
	r, err := New(root)
	if err != nil {
		t.Fatalf("new renderer: %v", err)
	}

	out, err := r.Render("templates/console/components/alert.html", map[string]any{
		"type":    "success",
		"message": "ok",
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	if !strings.Contains(out, `data-lucide="circle-check-big"`) && !strings.Contains(out, `data-lucide="check-circle"`) {
		t.Fatalf("expected lucide helper to add data-lucide attr")
	}
	if !strings.Contains(out, `class="lucide`) {
		t.Fatalf("expected lucide helper to inject class attribute")
	}
}
