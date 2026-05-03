package markdown

import (
	"bytes"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	mdast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	mdparser "github.com/yuin/goldmark/parser"
	mdhtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

type StoredLink struct {
	Original       string
	TargetClientID string
	TargetPath     string
	DisplayText    string
	LinkType       string
}

type AssetURLResolver func(vaultID, path string) string

type Renderer struct {
	markdown goldmark.Markdown
	assetURL AssetURLResolver
}

func New(assetURL AssetURLResolver) *Renderer {
	return &Renderer{
		markdown: goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			goldmark.WithParserOptions(mdparser.WithAutoHeadingID()),
			goldmark.WithRendererOptions(mdhtml.WithUnsafe(), mdhtml.WithHardWraps()),
		),
		assetURL: assetURL,
	}
}

type ParsedObsidianLink struct {
	Type    string
	Embed   bool
	Path    string
	Display string
	Anchor  string
}

func (r *Renderer) MdToHTML(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	trimmed := strings.TrimSpace(stripYAMLFrontMatter(content))
	if trimmed == "" {
		return ""
	}
	var buf bytes.Buffer
	if err := r.markdown.Convert([]byte(trimmed), &buf); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

func ParseObsidianLink(link string) ParsedObsidianLink {
	embed := strings.HasPrefix(link, "!")
	linkType := "link"
	if embed {
		linkType = "embed"
	}
	inner := strings.TrimPrefix(link, "!")
	inner = strings.TrimPrefix(inner, "[[")
	inner = strings.TrimSuffix(inner, "]]")
	pathPart := inner
	display := inner
	if parts := strings.SplitN(inner, "|", 2); len(parts) == 2 {
		pathPart = parts[0]
		display = parts[1]
	}
	path := pathPart
	anchor := ""
	if parts := strings.SplitN(pathPart, "#", 2); len(parts) == 2 {
		path = parts[0]
		anchor = parts[1]
	}
	return ParsedObsidianLink{
		Type:    linkType,
		Embed:   embed,
		Path:    strings.TrimSpace(path),
		Display: strings.TrimSpace(display),
		Anchor:  strings.TrimSpace(anchor),
	}
}

func ExtractMath(content string) (string, []MathFormula) {
	formulas := make([]MathFormula, 0)
	blockRe := regexp.MustCompile(`\$\$([^\$]+?)\$\$`)
	content = blockRe.ReplaceAllStringFunc(content, func(match string) string {
		parts := blockRe.FindStringSubmatch(match)
		idx := len(formulas)
		formulas = append(formulas, MathFormula{Type: "block", Formula: strings.TrimSpace(parts[1])})
		return fmt.Sprintf("MATHBLOCK%dMATHBLOCK", idx)
	})
	var out strings.Builder
	for i := 0; i < len(content); {
		if content[i] != '$' || (i+1 < len(content) && content[i+1] == '$') {
			out.WriteByte(content[i])
			i++
			continue
		}
		end := i + 1
		for end < len(content) && content[end] != '$' && content[end] != '\n' {
			end++
		}
		if end < len(content) && content[end] == '$' {
			formula := strings.TrimSpace(content[i+1 : end])
			idx := len(formulas)
			formulas = append(formulas, MathFormula{Type: "inline", Formula: formula})
			out.WriteString(fmt.Sprintf("MATHINLINE%dMATHINLINE", idx))
			i = end + 1
			continue
		}
		out.WriteByte(content[i])
		i++
	}
	return out.String(), formulas
}

type MathFormula struct {
	Type    string
	Formula string
}

func RestoreMath(htmlText string, formulas []MathFormula) string {
	result := htmlText
	for idx, formula := range formulas {
		placeholder := fmt.Sprintf("MATHINLINE%dMATHINLINE", idx)
		replacement := fmt.Sprintf(`<span class="math-inline">%s</span>`, formula.Formula)
		if formula.Type == "block" {
			placeholder = fmt.Sprintf("MATHBLOCK%dMATHBLOCK", idx)
			replacement = fmt.Sprintf(`<div class="math-block">%s</div>`, formula.Formula)
		}
		result = strings.ReplaceAll(result, placeholder, replacement)
	}
	return result
}

func (r *Renderer) ExtractTitle(content string) string {
	clean := strings.TrimSpace(stripYAMLFrontMatter(content))
	if clean == "" {
		return ""
	}
	doc := r.markdown.Parser().Parse(text.NewReader([]byte(clean)))
	var title string
	mdast.Walk(doc, func(node mdast.Node, entering bool) (mdast.WalkStatus, error) {
		if !entering {
			return mdast.WalkContinue, nil
		}
		heading, ok := node.(*mdast.Heading)
		if !ok || heading.Level != 1 {
			return mdast.WalkContinue, nil
		}
		title = strings.TrimSpace(string(heading.Text([]byte(clean))))
		return mdast.WalkStop, nil
	})
	return title
}

func (r *Renderer) ExtractDescription(content string, maxLength int) string {
	body := stripYAMLFrontMatter(content)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case strings.HasPrefix(line, "#"):
			continue
		case imageLineRe.MatchString(line):
			continue
		default:
			runes := []rune(line)
			if len(runes) > maxLength {
				if maxLength <= 0 {
					return ""
				}
				return string(runes[:maxLength])
			}
			return line
		}
	}
	return ""
}

func (r *Renderer) ReplaceObsidianLinks(content, vaultID string, links []StoredLink) string {
	masked, segments := maskCode(content)
	linkIndex := make(map[string]StoredLink, len(links))
	for _, link := range links {
		linkIndex[link.Original] = link
	}
	replaced := obsidianLinkRe.ReplaceAllStringFunc(masked, func(match string) string {
		parsed := ParseObsidianLink(match)
		normalized := normalizeAssetPath(parsed.Path)
		switch {
		case parsed.Embed && assetEmbed(normalized):
			return r.renderAssetEmbed(vaultID, normalized, parsed.Display)
		case !parsed.Embed && assetEmbed(normalized):
			return r.renderAssetLink(vaultID, normalized, parsed.Display)
		case linkIndex[match].TargetClientID != "":
			link := linkIndex[match]
			if link.LinkType == "embed" {
				return fmt.Sprintf(`<img src="/%s" alt="%s" class="obsidian-embed">`, link.TargetClientID, html.EscapeString(link.DisplayText))
			}
			href := "/" + link.TargetClientID
			if parsed.Anchor != "" {
				href += "#" + parsed.Anchor
			}
			return fmt.Sprintf(`<a href="%s" class="internal-link" data-note-id="%s">%s</a>`,
				href, link.TargetClientID, html.EscapeString(link.DisplayText))
		default:
			return fmt.Sprintf(`<span class="internal-link broken" title="Note not found: %s">%s</span>`,
				html.EscapeString(parsed.Path), html.EscapeString(parsed.Display))
		}
	})
	return unmaskCode(replaced, segments)
}

func (r *Renderer) RenderMarkdown(content, vaultID string, links []StoredLink) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	contentWithMath, formulas := ExtractMath(content)
	contentWithAssets := r.rewriteAssetLinks(contentWithMath, vaultID)
	contentWithLinks := r.ReplaceObsidianLinks(contentWithAssets, vaultID, links)
	return RestoreMath(r.MdToHTML(contentWithLinks), formulas)
}

func (r *Renderer) rewriteAssetLinks(content, vaultID string) string {
	masked, segments := maskCode(content)
	destinations := extractReferenceDefinitions(masked)
	result := inlineImageRe.ReplaceAllStringFunc(masked, func(match string) string {
		parts := inlineImageRe.FindStringSubmatch(match)
		dest, rest := splitLinkDestination(parts[2])
		normalized := normalizeAssetPath(dest)
		if assetEmbed(normalized) {
			return fmt.Sprintf("![%s](%s%s)", parts[1], r.assetURLFor(vaultID, normalized), rest)
		}
		return match
	})
	result = referenceImageRe.ReplaceAllStringFunc(result, func(match string) string {
		parts := referenceImageRe.FindStringSubmatch(match)
		destination := destinations[normalizeReferenceLabel(coalesce(parts[2], parts[1]))]
		if assetEmbed(destination) {
			return fmt.Sprintf("![%s](%s)", parts[1], r.assetURLFor(vaultID, destination))
		}
		return match
	})
	result = referenceDefinitionRe.ReplaceAllStringFunc(result, func(match string) string {
		parts := referenceDefinitionRe.FindStringSubmatch(match)
		dest, tail := splitLinkDestination(parts[2])
		normalized := normalizeAssetPath(dest)
		if assetEmbed(normalized) {
			return fmt.Sprintf("[%s]: %s%s", parts[1], r.assetURLFor(vaultID, normalized), tail)
		}
		return match
	})
	result = htmlMediaTagRe.ReplaceAllStringFunc(result, func(tag string) string {
		matches := htmlSrcRe.FindStringSubmatch(tag)
		raw := coalesce(matches[1], matches[2], matches[3])
		normalized := normalizeAssetPath(raw)
		if assetEmbed(normalized) {
			return htmlSrcRe.ReplaceAllString(tag, `src="`+r.assetURLFor(vaultID, normalized)+`"`)
		}
		return tag
	})
	return unmaskCode(result, segments)
}

func (r *Renderer) renderAssetEmbed(vaultID, path, display string) string {
	escaped := html.EscapeString(display)
	assetURL := r.assetURLFor(vaultID, path)
	lower := strings.ToLower(path)
	switch {
	case imageAssetRe.MatchString(lower):
		return fmt.Sprintf(`<img src="%s" alt="%s" class="asset-embed">`, assetURL, escaped)
	case strings.HasSuffix(lower, ".pdf"):
		return fmt.Sprintf(`<a href="%s" class="asset-link pdf-link">%s</a>`, assetURL, escaped)
	case audioAssetRe.MatchString(lower):
		return fmt.Sprintf(`<audio src="%s" controls class="asset-embed">%s</audio>`, assetURL, escaped)
	case videoAssetRe.MatchString(lower):
		return fmt.Sprintf(`<video src="%s" controls class="asset-embed">%s</video>`, assetURL, escaped)
	default:
		return fmt.Sprintf(`<a href="%s" class="asset-link">%s</a>`, assetURL, escaped)
	}
}

func (r *Renderer) renderAssetLink(vaultID, path, display string) string {
	return fmt.Sprintf(`<a href="%s" class="asset-link">%s</a>`, r.assetURLFor(vaultID, path), html.EscapeString(display))
}

func (r *Renderer) assetURLFor(vaultID, path string) string {
	if r.assetURL != nil {
		return r.assetURL(vaultID, path)
	}
	escaped := url.PathEscape(strings.TrimPrefix(path, "/"))
	escaped = strings.ReplaceAll(escaped, "%2F", "/")
	return "/storage/" + escaped
}

func stripYAMLFrontMatter(content string) string {
	if !strings.HasPrefix(content, "---") {
		return content
	}
	lines := strings.Split(content, "\n")
	for idx := 1; idx < len(lines); idx++ {
		if strings.HasPrefix(lines[idx], "---") {
			return strings.Join(lines[idx+1:], "\n")
		}
	}
	return content
}

func normalizeAssetPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.Trim(path, `"'<>`)
	path = strings.TrimPrefix(path, "./")
	path = strings.ReplaceAll(path, "\\", "/")
	if decoded, err := url.QueryUnescape(path); err == nil {
		path = decoded
	}
	for _, separator := range []string{"?", "#"} {
		if idx := strings.Index(path, separator); idx >= 0 {
			path = path[:idx]
		}
	}
	return path
}

func assetEmbed(path string) bool {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".md") {
		return false
	}
	return imageAssetRe.MatchString(lower) || strings.HasSuffix(lower, ".pdf") || audioAssetRe.MatchString(lower) || videoAssetRe.MatchString(lower)
}

func maskCode(content string) (string, []string) {
	segments := make([]string, 0)
	replacer := func(re *regexp.Regexp, input string) string {
		return re.ReplaceAllStringFunc(input, func(match string) string {
			idx := len(segments)
			segments = append(segments, match)
			return fmt.Sprintf("MB_CODE_SEGMENT_%d_MB", idx)
		})
	}
	content = replacer(codeBlockRe, content)
	content = replacer(inlineCodeRe, content)
	return content, segments
}

func unmaskCode(content string, segments []string) string {
	for idx, segment := range segments {
		content = strings.ReplaceAll(content, fmt.Sprintf("MB_CODE_SEGMENT_%d_MB", idx), segment)
	}
	return content
}

func splitLinkDestination(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "<") {
		if end := strings.Index(trimmed, ">"); end > 0 {
			dest := trimmed[1:end]
			return dest, trimmed[end+1:]
		}
		return "", ""
	}
	parts := strings.SplitN(trimmed, " ", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], " " + parts[1]
}

func extractReferenceDefinitions(content string) map[string]string {
	matches := referenceDefinitionRe.FindAllStringSubmatch(content, -1)
	out := make(map[string]string, len(matches))
	for _, match := range matches {
		dest, _ := splitLinkDestination(match[2])
		out[normalizeReferenceLabel(match[1])] = normalizeAssetPath(dest)
	}
	return out
}

func normalizeReferenceLabel(label string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(label))), " ")
}

func coalesce(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var (
	codeBlockRe           = regexp.MustCompile("(?s)(?:```|~~~).*?(?:```|~~~)")
	inlineCodeRe          = regexp.MustCompile("`[^`\n]*`")
	obsidianLinkRe        = regexp.MustCompile(`(!?)\[\[([^\]]+)\]\]`)
	inlineImageRe         = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
	referenceImageRe      = regexp.MustCompile(`!\[([^\]]*)\]\[([^\]]*)\]`)
	referenceDefinitionRe = regexp.MustCompile(`(?m)^\s*\[([^\]]+)\]:\s+(.+)$`)
	htmlMediaTagRe        = regexp.MustCompile(`<(img|audio|video|source)\b[^>]*>`)
	htmlSrcRe             = regexp.MustCompile(`src\s*=\s*(?:"([^"]+)"|'([^']+)'|([^\s>]+))`)
	imageAssetRe          = regexp.MustCompile(`.*\.(png|jpg|jpeg|gif|webp|svg|bmp|ico)$`)
	audioAssetRe          = regexp.MustCompile(`.*\.(mp3|ogg|wav)$`)
	videoAssetRe          = regexp.MustCompile(`.*\.(mp4|webm)$`)
	imageLineRe           = regexp.MustCompile(`^(!\[.*\]\(.*\)|!\[.*\]\[.*\]|!\[\[.*\]\])$`)
)
