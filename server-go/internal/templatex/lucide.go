package templatex

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	iconNameRe          = regexp.MustCompile(`^[a-z0-9-]+$`)
	legacyLucideAliases = map[string]string{
		"check-circle":   "circle-check-big",
		"alert-circle":   "circle-alert",
		"alert-triangle": "triangle-alert",
		"more-vertical":  "ellipsis-vertical",
	}
	svgOpenTagRe = regexp.MustCompile(`(?i)<svg\b`)
)

func (r *Renderer) lucideIcon(iconName string, attrs ...string) template.HTML {
	iconName = strings.TrimSpace(iconName)
	if iconName == "" || !iconNameRe.MatchString(iconName) {
		return ""
	}

	className := ""
	ariaLabel := ""
	if len(attrs) > 0 {
		className = strings.TrimSpace(attrs[0])
	}
	if len(attrs) > 1 {
		ariaLabel = strings.TrimSpace(attrs[1])
	}

	rawSVG, err := r.loadIcon(iconName)
	if err != nil || strings.TrimSpace(rawSVG) == "" {
		return ""
	}

	return template.HTML(injectSVGAttrs(rawSVG, iconName, className, ariaLabel))
}

func (r *Renderer) loadIcon(iconName string) (string, error) {
	name := iconName
	path := filepath.Join(r.root, "templates", "lucide", name+".svg")
	if _, err := os.Stat(path); err != nil {
		if alias, ok := legacyLucideAliases[iconName]; ok {
			name = alias
			path = filepath.Join(r.root, "templates", "lucide", name+".svg")
		}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read lucide icon %q: %w", iconName, err)
	}
	return string(b), nil
}

func injectSVGAttrs(svg, iconName, className, ariaLabel string) string {
	baseClass := normalizeClass(strings.Join([]string{"lucide", "lucide-" + iconName, className}, " "))
	attr := ""
	if baseClass != "" {
		attr += ` class="` + escapeAttr(baseClass) + `"`
	}
	attr += ` data-lucide="` + escapeAttr(iconName) + `"`
	if ariaLabel == "" {
		attr += ` aria-hidden="true" focusable="false"`
	} else {
		attr += ` role="img" aria-label="` + escapeAttr(ariaLabel) + `"`
	}
	return svgOpenTagRe.ReplaceAllString(svg, "<svg"+attr)
}

func normalizeClass(raw string) string {
	parts := strings.Fields(raw)
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return strings.Join(out, " ")
}

func escapeAttr(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		`"`, "&quot;",
		"<", "&lt;",
		">", "&gt;",
		"'", "&#x27;",
	)
	return replacer.Replace(s)
}
