package article

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestObsidianYAMLFrontMatter(t *testing.T) {
	p := filepath.Join(t.TempDir(), "note.md")
	writeFile(t, p, "---\ntitle: \"Title: YAML\"\ndraft: \"true\" # keep offline\ntags:\n  - go\n  - blog\ndescription: >-\n  A multiline\n  summary\n---\nBody\n---\nMore\n")
	a, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "Title: YAML" || !a.IsDraft() {
		t.Fatalf("YAML values misparsed: %+v", a.Meta)
	}
	if !reflect.DeepEqual(a.Tags, []string{"go", "blog"}) || a.Description != "A multiline summary" {
		t.Fatalf("YAML list/block lost: %+v", a)
	}
	if string(a.Content) != "Body\n---\nMore\n" {
		t.Fatalf("body changed: %q", a.Content)
	}
}

func TestRewritePreservesUnknownMetadata(t *testing.T) {
	p := filepath.Join(t.TempDir(), "note.md")
	writeFile(t, p, "---\ntitle: Hello\nupdated_time: 2026-10-12 12:00:00\npublish: false\ncustom:\n  nested: value\n---\nBody\n")
	a, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	a.Id = "stable"
	a.URL = "/post/stable"
	if err := RewriteFrontMatter(a); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	for _, want := range []string{"updated_time:", "publish: false", "nested: value", "Body"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("rewrite lost %q: %s", want, b)
		}
	}
}

func TestMalformedFrontMatterFailsWithoutLosingBody(t *testing.T) {
	for _, body := range []string{"---\ntitle: [broken\n---\nBody\n", "---\ntitle: Hello\nBody\n"} {
		p := filepath.Join(t.TempDir(), "bad.md")
		writeFile(t, p, body)
		if _, err := ParseFile(p); err == nil {
			t.Error("malformed front matter accepted")
		}
		got, _ := os.ReadFile(p)
		if string(got) != body {
			t.Error("parse modified source")
		}
	}
}
