package handlers

import (
	"testing"
	"time"

	"mdbrain.dev/internal/domain/model"
)

func TestParsePathIDsEmpty(t *testing.T) {
	if got := parsePathIDs("/"); got != nil {
		t.Fatalf("unexpected path ids: %#v", got)
	}
}

func TestParsePathIDsSplitMultipleNotes(t *testing.T) {
	got := parsePathIDs("/note-a+note-b")
	if len(got) != 2 || got[0] != "note-a" || got[1] != "note-b" {
		t.Fatalf("unexpected path ids: %#v", got)
	}
}

func TestBuildPushURLFromRootWithoutRootNote(t *testing.T) {
	if got := buildPushURL("/", "", "target-note", ""); got != "/target-note" {
		t.Fatalf("unexpected push url: %s", got)
	}
}

func TestBuildPushURLFromRootWithRootNote(t *testing.T) {
	if got := buildPushURL("/", "", "target-note", "root-note"); got != "/root-note+target-note" {
		t.Fatalf("unexpected push url: %s", got)
	}
}

func TestBuildPushURLTruncatesFromNote(t *testing.T) {
	if got := buildPushURL("https://notes.example.com/a+b+c?x=1", "b", "d", ""); got != "/a+b+d" {
		t.Fatalf("unexpected push url: %s", got)
	}
}

func TestExtractLogoHash(t *testing.T) {
	if got := extractLogoHash("site/logo/hash123.png"); got != "hash123" {
		t.Fatalf("unexpected logo hash: %s", got)
	}
}

func TestNoteListDataMapsClientIDsAndPath(t *testing.T) {
	now := time.Now().UTC()
	got := noteListData([]model.Note{{ClientID: "note-1", Path: "a.md", MTime: &now}})
	if len(got) != 1 {
		t.Fatalf("unexpected note list data length: %d", len(got))
	}
	item, ok := got[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected note list data item: %#v", got[0])
	}
	if item["client_id"] != "note-1" || item["path"] != "a.md" {
		t.Fatalf("unexpected note list data item: %#v", item)
	}
}
