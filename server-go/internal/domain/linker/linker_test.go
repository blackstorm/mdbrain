package linker

import "testing"

func TestExtractResolveDeduplicate(t *testing.T) {
	links := ExtractLinks("See [[Note A]] and ![[folder/Note B.md|B]] and [[Note A]]")
	if len(links) != 3 {
		t.Fatalf("expected 3 links, got %d", len(links))
	}

	resolved := ResolveLinks(links, []NoteRef{
		{ClientID: "a", Path: "Note A.md"},
		{ClientID: "b", Path: "folder/Note B.md"},
	})
	deduped := DeduplicateByTarget(resolved)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduped links, got %d", len(deduped))
	}
	if deduped[0].TargetClientID != "a" || deduped[1].TargetClientID != "b" {
		t.Fatalf("unexpected resolved links: %#v", deduped)
	}
}
