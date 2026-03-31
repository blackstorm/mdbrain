package linker

import (
	"regexp"
	"strings"
)

type Link struct {
	Original string
	Path     string
	Display  string
	Anchor   string
	LinkType string
}

type NoteRef struct {
	ClientID string
	Path     string
}

type ResolvedLink struct {
	TargetClientID string
	TargetPath     string
	LinkType       string
	DisplayText    string
	Original       string
}

var linkPattern = regexp.MustCompile(`(!?)\[\[([^\]]+)\]\]`)

func ExtractLinks(content string) []Link {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	matches := linkPattern.FindAllStringSubmatch(content, -1)
	out := make([]Link, 0, len(matches))
	for _, match := range matches {
		inner := match[2]
		pathAnchorParts := strings.SplitN(inner, "|", 2)
		pathAnchor := strings.TrimSpace(pathAnchorParts[0])
		display := pathAnchor
		if len(pathAnchorParts) == 2 {
			display = strings.TrimSpace(pathAnchorParts[1])
		}
		path := pathAnchor
		anchor := ""
		if pieces := strings.SplitN(pathAnchor, "#", 2); len(pieces) == 2 {
			path = strings.TrimSpace(pieces[0])
			anchor = strings.TrimSpace(pieces[1])
		}
		out = append(out, Link{
			Original: match[0],
			Path:     strings.TrimSpace(path),
			Display:  display,
			Anchor:   anchor,
			LinkType: map[bool]string{true: "embed", false: "link"}[match[1] == "!"],
		})
	}
	return out
}

func ResolveLinks(links []Link, notes []NoteRef) []ResolvedLink {
	index := buildNoteIndex(notes)
	out := make([]ResolvedLink, 0, len(links))
	for _, link := range links {
		target := findNote(link.Path, index)
		out = append(out, ResolvedLink{
			TargetClientID: target.ClientID,
			TargetPath:     link.Path,
			LinkType:       link.LinkType,
			DisplayText:    link.Display,
			Original:       link.Original,
		})
	}
	return out
}

func DeduplicateByTarget(links []ResolvedLink) []ResolvedLink {
	seen := make(map[string]struct{}, len(links))
	out := make([]ResolvedLink, 0, len(links))
	for _, link := range links {
		key := link.TargetClientID
		if key == "" {
			key = link.TargetPath
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, link)
	}
	return out
}

type noteIndex struct {
	byFullPath map[string]NoteRef
	byFilename map[string]NoteRef
}

func buildNoteIndex(notes []NoteRef) noteIndex {
	full := make(map[string]NoteRef, len(notes))
	file := make(map[string]NoteRef, len(notes))
	for _, note := range notes {
		normalized := normalizePath(note.Path)
		full[normalized] = note
		file[filename(normalized)] = note
	}
	return noteIndex{byFullPath: full, byFilename: file}
}

func findNote(path string, index noteIndex) NoteRef {
	normalized := normalizePath(path)
	if note, ok := index.byFullPath[normalized]; ok {
		return note
	}
	return index.byFilename[filename(normalized)]
}

func normalizePath(path string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(path), ".md"))
}

func filename(path string) string {
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
