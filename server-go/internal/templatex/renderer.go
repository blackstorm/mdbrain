package templatex

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

type Renderer struct {
	root  string
	mu    sync.RWMutex
	cache map[string]*template.Template
}

func New(root string) (*Renderer, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve template root: %w", err)
	}
	if _, err := os.Stat(absRoot); err != nil {
		return nil, fmt.Errorf("stat template root: %w", err)
	}
	return &Renderer{
		root:  absRoot,
		cache: map[string]*template.Template{},
	}, nil
}

func (r *Renderer) Render(templatePath string, data map[string]any) (string, error) {
	tpl, err := r.templateFor(templatePath)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, templatePath, normalizeAny(data)); err != nil {
		return "", fmt.Errorf("execute template %q: %w", templatePath, err)
	}
	return buf.String(), nil
}

func (r *Renderer) templateFor(templatePath string) (*template.Template, error) {
	r.mu.RLock()
	if tpl, ok := r.cache[templatePath]; ok {
		r.mu.RUnlock()
		return tpl, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	if tpl, ok := r.cache[templatePath]; ok {
		return tpl, nil
	}

	tpl, err := r.build(templatePath)
	if err != nil {
		return nil, err
	}
	r.cache[templatePath] = tpl
	return tpl, nil
}

func (r *Renderer) build(templatePath string) (*template.Template, error) {
	sources, order, err := r.collect(templatePath)
	if err != nil {
		return nil, err
	}

	tpl := template.New(templatePath).Funcs(template.FuncMap{
		"defaultVal": defaultVal,
		"iter":       iter,
		"length":     length,
		"lower":      strings.ToLower,
		"lucideIcon": r.lucideIcon,
		"notEmpty":   notEmpty,
		"safeHTML":   safeHTML,
		"scope":      scope,
		"value":      value,
	})

	for _, path := range order {
		transformed, err := transformTemplateSource(path, sources[path])
		if err != nil {
			return nil, fmt.Errorf("transform template %q: %w", path, err)
		}
		if _, err := tpl.Parse(transformed); err != nil {
			return nil, fmt.Errorf("parse template %q: %w", path, err)
		}
	}

	return tpl, nil
}

func (r *Renderer) collect(templatePath string) (map[string]string, []string, error) {
	sources := map[string]string{}
	order := make([]string, 0, 8)
	visiting := map[string]bool{}

	var visit func(string) error
	visit = func(path string) error {
		if _, ok := sources[path]; ok {
			return nil
		}
		if visiting[path] {
			return fmt.Errorf("cyclic template dependency: %s", path)
		}
		visiting[path] = true
		defer delete(visiting, path)

		fullPath := filepath.Join(r.root, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", fullPath, err)
		}
		raw := string(content)
		for _, dep := range referencedTemplates(raw) {
			if err := visit(dep); err != nil {
				return err
			}
		}
		sources[path] = raw
		order = append(order, path)
		return nil
	}

	if err := visit(templatePath); err != nil {
		return nil, nil, err
	}
	return sources, order, nil
}

func referencedTemplates(src string) []string {
	seen := map[string]struct{}{}
	deps := make([]string, 0, 4)

	for i := 0; i < len(src); {
		next := strings.Index(src[i:], "{%")
		if next < 0 {
			break
		}
		next += i
		end := strings.Index(src[next+2:], "%}")
		if end < 0 {
			break
		}
		end += next + 2

		tag := strings.TrimSpace(src[next+2 : end])
		switch {
		case strings.HasPrefix(tag, "extends "):
			if dep, ok := extractTemplatePath(tag[len("extends "):]); ok {
				if _, exists := seen[dep]; !exists {
					seen[dep] = struct{}{}
					deps = append(deps, dep)
				}
			}
		case strings.HasPrefix(tag, "include "):
			if dep, ok := extractTemplatePath(tag[len("include "):]); ok {
				if _, exists := seen[dep]; !exists {
					seen[dep] = struct{}{}
					deps = append(deps, dep)
				}
			}
		}

		i = end + 2
	}

	return deps
}

func transformTemplateSource(templatePath, src string) (string, error) {
	parent := ""
	if found, ok := extractExtends(src); ok {
		parent = found
		src = removeExtends(src)
	}

	body, err := translateTemplateBody(src, parent != "")
	if err != nil {
		return "", err
	}

	var out strings.Builder
	out.WriteString(`{{define "`)
	out.WriteString(templatePath)
	out.WriteString(`"}}`)
	if parent != "" {
		out.WriteString(`{{template "`)
		out.WriteString(parent)
		out.WriteString(`" .}}`)
	} else {
		out.WriteString(body)
	}
	out.WriteString(`{{end}}`)
	if parent != "" {
		out.WriteString(body)
	}
	return out.String(), nil
}

func extractExtends(src string) (string, bool) {
	for i := 0; i < len(src); {
		next := strings.Index(src[i:], "{%")
		if next < 0 {
			return "", false
		}
		next += i
		end := strings.Index(src[next+2:], "%}")
		if end < 0 {
			return "", false
		}
		end += next + 2

		tag := strings.TrimSpace(src[next+2 : end])
		if strings.HasPrefix(tag, "extends ") {
			return extractTemplatePath(tag[len("extends "):])
		}
		if strings.TrimSpace(src[:next]) != "" {
			return "", false
		}
		i = end + 2
	}
	return "", false
}

func removeExtends(src string) string {
	for i := 0; i < len(src); {
		next := strings.Index(src[i:], "{%")
		if next < 0 {
			return src
		}
		next += i
		end := strings.Index(src[next+2:], "%}")
		if end < 0 {
			return src
		}
		end += next + 2

		tag := strings.TrimSpace(src[next+2 : end])
		if strings.HasPrefix(tag, "extends ") {
			return src[:next] + src[end+2:]
		}
		if strings.TrimSpace(src[:next]) != "" {
			return src
		}
		i = end + 2
	}
	return src
}

func extractTemplatePath(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return "", false
	}
	if raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	unquoted, err := strconv.Unquote(raw)
	if err != nil {
		return "", false
	}
	return unquoted, true
}

func translateTemplateBody(src string, extending bool) (string, error) {
	var out strings.Builder

	for i := 0; i < len(src); {
		next := strings.IndexByte(src[i:], '{')
		if next < 0 {
			out.WriteString(src[i:])
			break
		}
		next += i
		out.WriteString(src[i:next])

		switch {
		case strings.HasPrefix(src[next:], "{{"):
			end := strings.Index(src[next+2:], "}}")
			if end < 0 {
				return "", fmt.Errorf("unclosed variable expression")
			}
			end += next + 2
			expr, err := translateExpression(src[next+2 : end])
			if err != nil {
				return "", err
			}
			out.WriteString("{{")
			out.WriteString(expr)
			out.WriteString("}}")
			i = end + 2
		case strings.HasPrefix(src[next:], "{%"):
			end := strings.Index(src[next+2:], "%}")
			if end < 0 {
				return "", fmt.Errorf("unclosed tag")
			}
			end += next + 2
			tag, err := translateTag(strings.TrimSpace(src[next+2:end]), extending)
			if err != nil {
				return "", err
			}
			out.WriteString(tag)
			i = end + 2
		case strings.HasPrefix(src[next:], "{#"):
			end := strings.Index(src[next+2:], "#}")
			if end < 0 {
				return "", fmt.Errorf("unclosed comment")
			}
			i = next + 2 + end + 2
		default:
			out.WriteByte(src[next])
			i = next + 1
		}
	}

	return out.String(), nil
}

func translateTag(tag string, extending bool) (string, error) {
	switch {
	case tag == "", strings.HasPrefix(tag, "extends "):
		return "", nil
	case tag == "else":
		return "{{else}}", nil
	case tag == "endif", tag == "endwith", tag == "endblock":
		return "{{end}}", nil
	case tag == "endfor":
		return "{{end}}{{end}}", nil
	case strings.HasPrefix(tag, "include "):
		path, ok := extractTemplatePath(tag[len("include "):])
		if !ok {
			return "", fmt.Errorf("invalid include tag: %s", tag)
		}
		return `{{template "` + path + `" .}}`, nil
	case strings.HasPrefix(tag, "block "):
		name := strings.TrimSpace(tag[len("block "):])
		if name == "" {
			return "", fmt.Errorf("missing block name")
		}
		if extending {
			return `{{define "` + name + `"}}`, nil
		}
		return `{{block "` + name + `" .}}`, nil
	case strings.HasPrefix(tag, "with "):
		assignments := splitFields(tag[len("with "):])
		if len(assignments) == 0 {
			return "", fmt.Errorf("invalid with tag: %s", tag)
		}
		var out strings.Builder
		out.WriteString("{{with scope .")
		for _, assignment := range assignments {
			parts := strings.SplitN(assignment, "=", 2)
			if len(parts) != 2 {
				return "", fmt.Errorf("invalid with assignment: %s", assignment)
			}
			key := strings.TrimSpace(parts[0])
			expr, err := translateExpression(parts[1])
			if err != nil {
				return "", err
			}
			out.WriteString(` "`)
			out.WriteString(key)
			out.WriteString(`" (`)
			out.WriteString(expr)
			out.WriteString(`)`)
		}
		out.WriteString("}}")
		return out.String(), nil
	case strings.HasPrefix(tag, "for "):
		name, expr, ok := strings.Cut(strings.TrimSpace(tag[len("for "):]), " in ")
		if !ok {
			return "", fmt.Errorf("invalid for tag: %s", tag)
		}
		name = strings.TrimSpace(name)
		expr, err := translateExpression(expr)
		if err != nil {
			return "", err
		}
		return `{{range $__index, $__item := iter (` + expr + `)}}{{with scope . "` + name + `" $__item}}`, nil
	case strings.HasPrefix(tag, "if "):
		expr, err := translateCondition(tag[len("if "):])
		if err != nil {
			return "", err
		}
		return "{{if " + expr + "}}", nil
	case strings.HasPrefix(tag, "elif "):
		expr, err := translateCondition(tag[len("elif "):])
		if err != nil {
			return "", err
		}
		return "{{else if " + expr + "}}", nil
	default:
		return "", fmt.Errorf("unsupported tag: %s", tag)
	}
}

func translateCondition(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	for _, op := range []string{"!=", "==", ">=", "<=", ">", "<", "="} {
		if left, right, ok := splitComparison(raw, op); ok {
			lv, err := translateExpression(left)
			if err != nil {
				return "", err
			}
			rv, err := translateExpression(right)
			if err != nil {
				return "", err
			}
			return comparisonFunc(op) + " (" + lv + ") (" + rv + ")", nil
		}
	}
	return translateExpression(raw)
}

func splitComparison(raw, op string) (string, string, bool) {
	inSingle := false
	inDouble := false
	depth := 0
	for i := 0; i <= len(raw)-len(op); i++ {
		ch := raw[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '(':
			if !inSingle && !inDouble {
				depth++
			}
		case ')':
			if !inSingle && !inDouble && depth > 0 {
				depth--
			}
		}
		if inSingle || inDouble || depth > 0 {
			continue
		}
		if strings.HasPrefix(raw[i:], op) {
			return strings.TrimSpace(raw[:i]), strings.TrimSpace(raw[i+len(op):]), true
		}
	}
	return "", "", false
}

func comparisonFunc(op string) string {
	switch op {
	case "!=", "ne":
		return "ne"
	case ">":
		return "gt"
	case "<":
		return "lt"
	case ">=":
		return "ge"
	case "<=":
		return "le"
	default:
		return "eq"
	}
}

func translateExpression(raw string) (string, error) {
	parts := splitTopLevel(raw, '|')
	if len(parts) == 0 {
		return `""`, nil
	}

	expr, err := translatePrimary(parts[0])
	if err != nil {
		return "", err
	}

	for _, filter := range parts[1:] {
		name, arg, _ := strings.Cut(strings.TrimSpace(filter), ":")
		name = strings.TrimSpace(name)
		switch name {
		case "safe":
			expr = "safeHTML (" + expr + ")"
		case "default":
			argExpr, err := translatePrimary(arg)
			if err != nil {
				return "", err
			}
			expr = "defaultVal (" + expr + ") (" + argExpr + ")"
		case "lower":
			expr = "lower (" + expr + ")"
		case "length":
			expr = "length (" + expr + ")"
		case "not-empty":
			expr = "notEmpty (" + expr + ")"
		default:
			return "", fmt.Errorf("unsupported filter: %s", name)
		}
	}

	return expr, nil
}

func translatePrimary(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return `""`, nil
	}

	if isQuotedLiteral(raw) {
		return normalizeQuotedLiteral(raw)
	}
	if raw == "true" || raw == "false" {
		return raw, nil
	}
	if _, err := strconv.Atoi(raw); err == nil {
		return raw, nil
	}

	if name, args, ok := parseCall(raw); ok {
		translatedArgs := make([]string, 0, len(args))
		for _, arg := range args {
			expr, err := translateExpression(arg)
			if err != nil {
				return "", err
			}
			translatedArgs = append(translatedArgs, "("+expr+")")
		}
		return callName(name) + " " + strings.Join(translatedArgs, " "), nil
	}

	return `value . "` + strings.TrimSpace(raw) + `"`, nil
}

func parseCall(raw string) (string, []string, bool) {
	open := strings.IndexByte(raw, '(')
	if open <= 0 || raw[len(raw)-1] != ')' {
		return "", nil, false
	}
	name := strings.TrimSpace(raw[:open])
	if name == "" {
		return "", nil, false
	}
	for _, r := range name {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return "", nil, false
		}
	}
	argsBody := strings.TrimSpace(raw[open+1 : len(raw)-1])
	if argsBody == "" {
		return name, nil, true
	}
	return name, splitTopLevel(argsBody, ','), true
}

func callName(name string) string {
	switch strings.ReplaceAll(name, "-", "_") {
	case "lucide_icon":
		return "lucideIcon"
	default:
		return strings.ReplaceAll(name, "-", "_")
	}
}

func splitTopLevel(raw string, sep rune) []string {
	var parts []string
	inSingle := false
	inDouble := false
	depth := 0
	start := 0

	for i, r := range raw {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '(':
			if !inSingle && !inDouble {
				depth++
			}
		case ')':
			if !inSingle && !inDouble && depth > 0 {
				depth--
			}
		default:
			if r == sep && !inSingle && !inDouble && depth == 0 {
				parts = append(parts, strings.TrimSpace(raw[start:i]))
				start = i + 1
			}
		}
	}

	parts = append(parts, strings.TrimSpace(raw[start:]))
	return parts
}

func splitFields(raw string) []string {
	var fields []string
	inSingle := false
	inDouble := false
	depth := 0
	start := -1

	flush := func(end int) {
		if start < 0 {
			return
		}
		field := strings.TrimSpace(raw[start:end])
		if field != "" {
			fields = append(fields, field)
		}
		start = -1
	}

	for i, r := range raw {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '(':
			if !inSingle && !inDouble {
				depth++
			}
		case ')':
			if !inSingle && !inDouble && depth > 0 {
				depth--
			}
		}

		if unicode.IsSpace(r) && !inSingle && !inDouble && depth == 0 {
			flush(i)
			continue
		}
		if start < 0 {
			start = i
		}
	}
	flush(len(raw))
	return fields
}

func isQuotedLiteral(raw string) bool {
	if len(raw) < 2 {
		return false
	}
	first := raw[0]
	last := raw[len(raw)-1]
	return (first == '"' && last == '"') || (first == '\'' && last == '\'')
}

func normalizeQuotedLiteral(raw string) (string, error) {
	if raw[0] == '"' {
		return raw, nil
	}
	return strconv.Quote(raw[1 : len(raw)-1]), nil
}

func normalizeAny(v any) any {
	switch x := v.(type) {
	case nil:
		return map[string]any{}
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, vv := range x {
			out[k] = normalizeAny(vv)
		}
		return out
	case []map[string]any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, normalizeAny(item))
		}
		return out
	case []any:
		out := make([]any, 0, len(x))
		for _, item := range x {
			out = append(out, normalizeAny(item))
		}
		return out
	default:
		rv := reflect.ValueOf(v)
		if !rv.IsValid() {
			return nil
		}
		if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
			out := make([]any, 0, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				out = append(out, normalizeAny(rv.Index(i).Interface()))
			}
			return out
		}
		return v
	}
}

func value(ctx any, path string) any {
	current := ctx
	if strings.TrimSpace(path) == "" {
		return current
	}

	for _, part := range strings.Split(path, ".") {
		current = resolvePart(current, part)
		if current == nil {
			return nil
		}
	}
	return current
}

func resolvePart(v any, part string) any {
	if v == nil {
		return nil
	}

	rv := reflect.ValueOf(v)
	for rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}

	if idx, err := strconv.Atoi(part); err == nil {
		if (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) && idx >= 0 && idx < rv.Len() {
			return rv.Index(idx).Interface()
		}
		return nil
	}

	switch rv.Kind() {
	case reflect.Map:
		for _, key := range lookupKeys(part) {
			mv := rv.MapIndex(reflect.ValueOf(key))
			if mv.IsValid() {
				return mv.Interface()
			}
		}
	case reflect.Struct:
		for _, fieldName := range lookupFieldNames(part) {
			field := rv.FieldByName(fieldName)
			if field.IsValid() && field.CanInterface() {
				return field.Interface()
			}
		}
	case reflect.Slice, reflect.Array:
		return nil
	}

	if m, ok := v.(map[string]any); ok {
		for _, key := range lookupKeys(part) {
			if found, exists := m[key]; exists {
				return found
			}
		}
	}

	return nil
}

func lookupKeys(part string) []string {
	keys := []string{part}
	if alt := strings.ReplaceAll(part, "_", "-"); alt != part {
		keys = append(keys, alt)
	}
	if alt := strings.ReplaceAll(part, "-", "_"); alt != part {
		keys = append(keys, alt)
	}
	return keys
}

func lookupFieldNames(part string) []string {
	raw := strings.FieldsFunc(part, func(r rune) bool {
		return r == '-' || r == '_'
	})
	var b strings.Builder
	for _, segment := range raw {
		if segment == "" {
			continue
		}
		b.WriteString(strings.ToUpper(segment[:1]))
		if len(segment) > 1 {
			b.WriteString(segment[1:])
		}
	}

	out := []string{b.String()}
	if part != "" {
		out = append(out, part)
	}
	return out
}

func scope(base any, kv ...any) map[string]any {
	out := map[string]any{}
	if existing, ok := normalizeAny(base).(map[string]any); ok {
		for k, v := range existing {
			out[k] = v
		}
	}

	for i := 0; i+1 < len(kv); i += 2 {
		key, _ := kv[i].(string)
		if key == "" {
			continue
		}
		out[key] = normalizeAny(kv[i+1])
	}
	return out
}

func defaultVal(v, fallback any) any {
	if v == nil {
		return fallback
	}
	switch x := v.(type) {
	case string:
		if x == "" {
			return fallback
		}
	case *string:
		if x == nil || *x == "" {
			return fallback
		}
	}
	return v
}

func notEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len() > 0
	}
	return true
}

func length(v any) int {
	if v == nil {
		return 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len()
	default:
		return 0
	}
}

func iter(v any) any {
	if v == nil {
		return []any{}
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice:
		return v
	default:
		return []any{}
	}
}

func safeHTML(v any) template.HTML {
	switch x := v.(type) {
	case nil:
		return ""
	case template.HTML:
		return x
	case string:
		return template.HTML(x)
	default:
		return template.HTML(fmt.Sprint(x))
	}
}
