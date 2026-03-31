package templatex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppBaseTemplateIncludesHighlightAssets(t *testing.T) {
	root := filepath.Join(repoRoot(t), "server", "resources")
	content, err := os.ReadFile(filepath.Join(root, "templates", "app", "base.html"))
	if err != nil {
		t.Fatalf("read app base template: %v", err)
	}

	html := string(content)
	if !strings.Contains(html, "/publics/app/css/highlight-github-dark.min.css") {
		t.Fatalf("expected highlight css asset in app base template")
	}
	if !strings.Contains(html, "/publics/app/js/highlight.min.js") {
		t.Fatalf("expected highlight js asset in app base template")
	}
}

func TestConsoleTemplatesDeclareNoIndex(t *testing.T) {
	root := filepath.Join(repoRoot(t), "server", "resources")
	templates := []string{
		"templates/console/base.html",
		"templates/console/login.html",
		"templates/console/init.html",
		"templates/console/vaults.html",
	}

	for _, name := range templates {
		t.Run(name, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
			if err != nil {
				t.Fatalf("read template %s: %v", name, err)
			}
			if !strings.Contains(string(content), `<meta name="robots" content="noindex, nofollow">`) {
				t.Fatalf("expected noindex meta tag in %s", name)
			}
		})
	}
}
