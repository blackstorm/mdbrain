package markdown

import (
	"strings"
	"testing"
)

func TestMdToHTMLEmptyContent(t *testing.T) {
	if got := New(nil).MdToHTML("   "); got != "" {
		t.Fatalf("unexpected html: %q", got)
	}
}

func TestMdToHTMLFrontMatterOnlyReturnsEmpty(t *testing.T) {
	if got := New(nil).MdToHTML("---\ntitle: Test\n---\n"); got != "" {
		t.Fatalf("unexpected html: %q", got)
	}
}

func TestParseObsidianLinkBasic(t *testing.T) {
	parsed := ParseObsidianLink("[[My Note]]")
	if parsed.Type != "link" || parsed.Path != "My Note" || parsed.Display != "My Note" || parsed.Anchor != "" {
		t.Fatalf("unexpected parsed link: %#v", parsed)
	}
}

func TestParseObsidianLinkEmbedAnchorAndDisplay(t *testing.T) {
	parsed := ParseObsidianLink("![[Folder/Note#Section|Shown]]")
	if !parsed.Embed || parsed.Type != "embed" || parsed.Path != "Folder/Note" || parsed.Anchor != "Section" || parsed.Display != "Shown" {
		t.Fatalf("unexpected parsed link: %#v", parsed)
	}
}

func TestExtractMathInlineAndBlock(t *testing.T) {
	content, formulas := ExtractMath("inline $x + y$ and $$z^2$$")
	if !strings.Contains(content, "MATHINLINE1MATHINLINE") || !strings.Contains(content, "MATHBLOCK0MATHBLOCK") {
		t.Fatalf("unexpected masked content: %s", content)
	}
	if len(formulas) != 2 || formulas[0].Type != "block" || formulas[1].Type != "inline" {
		t.Fatalf("unexpected formulas: %#v", formulas)
	}
}

func TestRestoreMathInlineAndBlock(t *testing.T) {
	got := RestoreMath("MATHINLINE0MATHINLINE MATHBLOCK1MATHBLOCK", []MathFormula{
		{Type: "inline", Formula: "x+y"},
		{Type: "block", Formula: "z^2"},
	})
	if !strings.Contains(got, `<span class="math-inline">x+y</span>`) || !strings.Contains(got, `<div class="math-block">z^2</div>`) {
		t.Fatalf("unexpected restored html: %s", got)
	}
}

func TestExtractTitleWithoutHeadingReturnsEmpty(t *testing.T) {
	if got := New(nil).ExtractTitle("Paragraph only"); got != "" {
		t.Fatalf("unexpected title: %q", got)
	}
}

func TestExtractDescriptionTruncatesLongLine(t *testing.T) {
	got := New(nil).ExtractDescription("# Title\n\nThis description is definitely longer than ten characters.", 10)
	if got != "This descr" {
		t.Fatalf("unexpected description: %q", got)
	}
}

func TestExtractDescriptionTruncatesOnRuneBoundary(t *testing.T) {
	got := New(nil).ExtractDescription("# 标题\n\n你好世界欢迎使用 Mdbrain。", 5)
	if got != "你好世界欢" {
		t.Fatalf("unexpected utf-8-safe description: %q", got)
	}
}

func TestReplaceObsidianLinksBrokenLink(t *testing.T) {
	got := New(nil).ReplaceObsidianLinks("[[Missing Note]]", "vault-1", nil)
	if !strings.Contains(got, `class="internal-link broken"`) {
		t.Fatalf("unexpected html: %s", got)
	}
}

func TestReplaceObsidianLinksInternalLinkWithAnchor(t *testing.T) {
	got := New(nil).ReplaceObsidianLinks("[[Target#Section|Open]]", "vault-1", []StoredLink{
		{Original: "[[Target#Section|Open]]", TargetClientID: "note-1", TargetPath: "Target", DisplayText: "Open", LinkType: "link"},
	})
	if !strings.Contains(got, `href="/note-1#Section"`) || !strings.Contains(got, `>Open</a>`) {
		t.Fatalf("unexpected html: %s", got)
	}
}

func TestReplaceObsidianLinksAssetEmbedImage(t *testing.T) {
	got := New(nil).ReplaceObsidianLinks("![[assets/photo.png]]", "vault-1", nil)
	if !strings.Contains(got, `<img src="/storage/assets/photo.png"`) {
		t.Fatalf("unexpected html: %s", got)
	}
}

func TestReplaceObsidianLinksAssetLinkPDF(t *testing.T) {
	got := New(nil).ReplaceObsidianLinks("[[docs/file.pdf|Manual]]", "vault-1", nil)
	if !strings.Contains(got, `class="asset-link"`) || !strings.Contains(got, `Manual`) {
		t.Fatalf("unexpected html: %s", got)
	}
}

func TestRenderMarkdownPreservesInlineCode(t *testing.T) {
	got := New(nil).RenderMarkdown("`[[Do Not Link]]`", "vault-1", nil)
	if strings.Contains(got, `class="internal-link"`) {
		t.Fatalf("expected code span to remain untouched: %s", got)
	}
}

func TestRewriteAssetLinksReferenceImage(t *testing.T) {
	got := New(nil).RenderMarkdown("![Alt][img]\n\n[img]: assets/file.png", "vault-1", nil)
	if !strings.Contains(got, `/storage/assets/file.png`) {
		t.Fatalf("unexpected html: %s", got)
	}
}

func TestRewriteAssetLinksHTMLMediaTag(t *testing.T) {
	got := New(nil).RenderMarkdown(`<video src="assets/movie.mp4"></video>`, "vault-1", nil)
	if !strings.Contains(got, `src="/storage/assets/movie.mp4"`) {
		t.Fatalf("unexpected html: %s", got)
	}
}

func TestSplitLinkDestinationWithAngleBrackets(t *testing.T) {
	dest, tail := splitLinkDestination("<assets/file with spaces.png> \"Title\"")
	if dest != "assets/file with spaces.png" || tail != ` "Title"` {
		t.Fatalf("unexpected split destination: dest=%q tail=%q", dest, tail)
	}
}

func TestSplitLinkDestinationWithoutSuffix(t *testing.T) {
	dest, tail := splitLinkDestination("assets/file.png")
	if dest != "assets/file.png" || tail != "" {
		t.Fatalf("unexpected split destination: dest=%q tail=%q", dest, tail)
	}
}

func TestExtractReferenceDefinitionsNormalizesLabel(t *testing.T) {
	got := extractReferenceDefinitions("[My   Label]: ./assets/file.png")
	if got["my label"] != "assets/file.png" {
		t.Fatalf("unexpected definitions: %#v", got)
	}
}

func TestNormalizeAssetPathDecodesAndStripsSuffixes(t *testing.T) {
	got := normalizeAssetPath(`./assets/My%20File.png?download=1#top`)
	if got != "assets/My File.png" {
		t.Fatalf("unexpected normalized asset path: %q", got)
	}
}

func TestAssetEmbedRecognizesSupportedMedia(t *testing.T) {
	for _, path := range []string{"image.png", "doc.pdf", "audio.mp3", "movie.mp4"} {
		if !assetEmbed(path) {
			t.Fatalf("expected asset path to embed: %s", path)
		}
	}
}

func TestAssetEmbedIgnoresMarkdownFiles(t *testing.T) {
	if assetEmbed("note.md") {
		t.Fatal("expected markdown file to be ignored as asset embed")
	}
}

func TestExtractDescriptionEmptyContent(t *testing.T) {
	if got := New(nil).ExtractDescription("", 10); got != "" {
		t.Fatalf("unexpected description: %q", got)
	}
}

func TestStripYAMLFrontMatterWithoutClosingFenceReturnsOriginal(t *testing.T) {
	content := "---\ntitle: Test\nbody"
	if got := stripYAMLFrontMatter(content); got != content {
		t.Fatalf("unexpected stripped content: %q", got)
	}
}

func TestNormalizeAssetPathStripsQuotes(t *testing.T) {
	if got := normalizeAssetPath(`"assets/file.png"`); got != "assets/file.png" {
		t.Fatalf("unexpected normalized asset path: %q", got)
	}
}

func TestNormalizeReferenceLabelCollapsesWhitespace(t *testing.T) {
	if got := normalizeReferenceLabel("  My   Label  "); got != "my label" {
		t.Fatalf("unexpected normalized reference label: %q", got)
	}
}

func TestFirstNonEmptyReturnsFirstValue(t *testing.T) {
	if got := firstNonEmpty("", "a", "b"); got != "a" {
		t.Fatalf("unexpected first non-empty markdown value: %q", got)
	}
}

func TestOrReturnsFallbackWhenEmpty(t *testing.T) {
	if got := or("", "fallback"); got != "fallback" {
		t.Fatalf("unexpected or result: %q", got)
	}
}

func TestOrReturnsValueWhenPresent(t *testing.T) {
	if got := or("value", "fallback"); got != "value" {
		t.Fatalf("unexpected or result: %q", got)
	}
}

func TestSplitLinkDestinationInvalidBracketReturnsEmpty(t *testing.T) {
	dest, tail := splitLinkDestination("<broken")
	if dest != "" || tail != "" {
		t.Fatalf("unexpected split destination: dest=%q tail=%q", dest, tail)
	}
}

func TestReplaceObsidianLinksEmbedResolvedNoteRendersImage(t *testing.T) {
	got := New(nil).ReplaceObsidianLinks("![[Diagram Note]]", "vault-1", []StoredLink{
		{Original: "![[Diagram Note]]", TargetClientID: "note-embed-1", TargetPath: "Diagram Note", DisplayText: "Diagram Note", LinkType: "embed"},
	})
	if !strings.Contains(got, `src="/note-embed-1"`) {
		t.Fatalf("unexpected html: %s", got)
	}
}

func TestRenderMarkdownEmptyContent(t *testing.T) {
	if got := New(nil).RenderMarkdown("", "vault-1", nil); got != "" {
		t.Fatalf("unexpected html: %q", got)
	}
}

func TestRewriteAssetLinksLeavesNonAssetLinkUntouched(t *testing.T) {
	content := New(nil).RenderMarkdown("![Alt](notes/doc.md)", "vault-1", nil)
	if !strings.Contains(content, `src="notes/doc.md"`) && !strings.Contains(content, `href="notes/doc.md"`) && !strings.Contains(content, "notes/doc.md") {
		t.Fatalf("expected markdown note link to stay untouched, got: %s", content)
	}
}

func TestAssetURLForUsesCustomResolver(t *testing.T) {
	renderer := New(func(vaultID, path string) string {
		return "/cdn/" + vaultID + "/" + path
	})
	if got := renderer.assetURLFor("vault-1", "assets/file.png"); got != "/cdn/vault-1/assets/file.png" {
		t.Fatalf("unexpected asset url: %s", got)
	}
}
