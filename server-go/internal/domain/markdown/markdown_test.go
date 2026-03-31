package markdown

import (
	"strings"
	"testing"
)

func TestRenderMarkdown(t *testing.T) {
	renderer := New(nil)
	html := renderer.RenderMarkdown("# Test Note\n\nThis is a [[Other Note]] with $x^2$.\n\n$$E = mc^2$$", "vault", []StoredLink{
		{Original: "[[Other Note]]", TargetClientID: "client-1", TargetPath: "Other Note", DisplayText: "Other Note", LinkType: "link"},
	})
	if !strings.Contains(html, "<h1") || !strings.Contains(html, "Test Note") {
		t.Fatalf("expected heading, got %s", html)
	}
	if !strings.Contains(html, `href="/client-1"`) {
		t.Fatalf("expected internal link, got %s", html)
	}
	if !strings.Contains(html, "math-inline") || !strings.Contains(html, "math-block") {
		t.Fatalf("expected math markers, got %s", html)
	}
}

func TestAssetLinkRewrite(t *testing.T) {
	renderer := New(nil)
	html := renderer.RenderMarkdown("![Alt](assets/inline.png)\n\n<img src=\"assets/html.png\" />", "vault", nil)
	if !strings.Contains(html, `/storage/assets/inline.png`) || !strings.Contains(html, `/storage/assets/html.png`) {
		t.Fatalf("expected asset urls, got %s", html)
	}
}

func TestExtractTitleAndDescription(t *testing.T) {
	renderer := New(nil)
	title := renderer.ExtractTitle("---\ntitle: fake\n---\n# Real Title\n\nBody")
	if title != "Real Title" {
		t.Fatalf("unexpected title: %s", title)
	}
	desc := renderer.ExtractDescription("# Title\n\n![[image.png]]\n\nThis is the description.", 100)
	if desc != "This is the description." {
		t.Fatalf("unexpected description: %s", desc)
	}
}
