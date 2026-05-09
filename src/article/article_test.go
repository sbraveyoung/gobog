package article

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseFileExposesFrontMatter(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello.md")
	writeFile(t, p, `---
title: Hello
description: a desc
author: smart
create_time: 2026-04-25 09:00:00
tags: go, blog, networking
id: deadbeef
url: /post/deadbeef
---
some body
with multiple words
`)

	a, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "Hello" || a.Description != "a desc" || a.Author != "smart" {
		t.Errorf("front matter not parsed: %+v", a.Meta)
	}
	if a.URL != "/post/deadbeef" || a.Id != "deadbeef" {
		t.Errorf("url/id mismatch: %+v", a.Meta)
	}
	want := []string{"go", "blog", "networking"}
	if !reflect.DeepEqual(a.Tags, want) {
		t.Errorf("Tags = %v, want %v", a.Tags, want)
	}
	if a.WordCount == 0 || a.ReadingTimeMin == 0 {
		t.Errorf("expected word count + reading time, got %+v", a)
	}
	if a.Summary != "a desc" {
		t.Errorf("Summary should fall back to Description, got %q", a.Summary)
	}
	if a.Source != p {
		t.Errorf("Source = %q, want %q", a.Source, p)
	}
}

// TestRewriteFrontMatterRoundTrip verifies the persist-back path used by
// the blog scanner: parse → fill in defaults → RewriteFrontMatter → parse
// again should observe the same Meta.
func TestRewriteFrontMatterRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "no-meta.md")
	writeFile(t, p, "just a body, nothing else\n")

	a, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	a.Title = "no-meta"
	a.CreateTime = "2026-04-25 09:00:00"
	a.Id = "abc123"
	a.URL = "/post/abc123"
	if err := RewriteFrontMatter(a); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !startsWith(got, "---\n") {
		t.Errorf("file does not start with front-matter block:\n%s", got)
	}

	a2, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if a2.Title != a.Title || a2.Id != a.Id || a2.URL != a.URL || a2.CreateTime != a.CreateTime {
		t.Errorf("round-trip lost data: before=%+v after=%+v", a.Meta, a2.Meta)
	}
}

func startsWith(b []byte, s string) bool {
	if len(b) < len(s) {
		return false
	}
	return string(b[:len(s)]) == s
}

func TestParseTags(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"go", []string{"go"}},
		{"go,blog", []string{"go", "blog"}},
		{" go , blog , , ", []string{"go", "blog"}},
	}
	for _, c := range cases {
		got := parseTags(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseTags(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCountWordsMixedCJKAndAscii(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"hello world", 2},
		{"hello, world!", 2},
		{"你好世界", 4},
		{"hello 你好", 3},
		{"   ", 0},
	}
	for _, c := range cases {
		if got := countWords([]byte(c.in)); got != c.want {
			t.Errorf("countWords(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestArticlesSortingGroupsFirstThenDateDesc(t *testing.T) {
	leafA := &Article{}
	leafA.CreateTime = "2026-04-25 10:00:00"
	leafB := &Article{}
	leafB.CreateTime = "2026-04-24 10:00:00"
	group := &Article{SubArticle: Articles{leafA}}
	group.CreateTime = "2026-04-23 10:00:00"

	list := Articles{leafA, leafB, group}
	sort.Sort(list)

	if list[0] != group {
		t.Errorf("groups should sort first; got order: %v", titles(list))
	}
	if list[1] != leafA || list[2] != leafB {
		t.Errorf("leaves should sort by CreateTime desc; got order: %v", titles(list))
	}
}

func TestArticlesSortingPinnedAboveDateOrder(t *testing.T) {
	newer := &Article{}
	newer.Title = "newer"
	newer.CreateTime = "2026-04-25 10:00:00"
	older := &Article{}
	older.Title = "older"
	older.CreateTime = "2026-04-20 10:00:00"
	pinned := &Article{}
	pinned.Title = "pinned"
	pinned.CreateTime = "2026-04-10 10:00:00"
	pinned.Pin = "true"

	list := Articles{newer, older, pinned}
	sort.Sort(list)

	if list[0] != pinned {
		t.Fatalf("pinned should be first regardless of date, got order: %v", titlesOf(list))
	}
	if list[1] != newer || list[2] != older {
		t.Errorf("non-pinned should still sort by date desc, got: %v", titlesOf(list))
	}
}

func TestArticlesSortingMultiplePinned(t *testing.T) {
	pinNew := &Article{}
	pinNew.Title = "pin-new"
	pinNew.Pin = "yes"
	pinNew.CreateTime = "2026-04-25 10:00:00"
	pinOld := &Article{}
	pinOld.Title = "pin-old"
	pinOld.Pin = "1"
	pinOld.CreateTime = "2026-01-01 10:00:00"
	leaf := &Article{}
	leaf.Title = "regular"
	leaf.CreateTime = "2026-04-26 10:00:00"

	list := Articles{leaf, pinOld, pinNew}
	sort.Sort(list)

	if list[0].Title != "pin-new" || list[1].Title != "pin-old" {
		t.Errorf("among pinned, newer should win, got: %v", titlesOf(list))
	}
	if list[2] != leaf {
		t.Errorf("regular post should sort last among these, got: %v", titlesOf(list))
	}
}

func titlesOf(a Articles) []string {
	out := make([]string, len(a))
	for i, x := range a {
		out[i] = x.Title
	}
	return out
}

func TestVisibilityFlags(t *testing.T) {
	cases := []struct {
		raw                       string
		draft, hidden, priv, pin  bool
		setter                    func(*Article, string)
		check                     func(*Article) bool
		name                      string
	}{
		{"true", true, false, false, false, func(a *Article, v string) { a.Draft = v }, (*Article).IsDraft, "draft=true"},
		{"True", true, false, false, false, func(a *Article, v string) { a.Draft = v }, (*Article).IsDraft, "draft=True"},
		{"YES", true, false, false, false, func(a *Article, v string) { a.Draft = v }, (*Article).IsDraft, "draft=YES"},
		{"on", true, false, false, false, func(a *Article, v string) { a.Draft = v }, (*Article).IsDraft, "draft=on"},
		{"false", false, false, false, false, func(a *Article, v string) { a.Draft = v }, (*Article).IsDraft, "draft=false"},
		{"", false, false, false, false, func(a *Article, v string) { a.Draft = v }, (*Article).IsDraft, "draft=empty"},
		{"true", false, true, false, false, func(a *Article, v string) { a.Hidden = v }, (*Article).IsHidden, "hidden=true"},
		{"yes", false, false, true, false, func(a *Article, v string) { a.Private = v }, (*Article).IsPrivate, "private=yes"},
		{"1", false, false, false, true, func(a *Article, v string) { a.Pin = v }, (*Article).IsPinned, "pin=1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := &Article{}
			c.setter(a, c.raw)
			if got := c.check(a); got != (c.draft || c.hidden || c.priv || c.pin) {
				t.Errorf("%s: check returned %v", c.name, got)
			}
		})
	}
}

func titles(a Articles) []string {
	out := make([]string, len(a))
	for i, x := range a {
		if len(x.SubArticle) > 0 {
			out[i] = "group"
		} else {
			out[i] = x.CreateTime
		}
	}
	return out
}

func TestCachedHTMLIsLazyAndStable(t *testing.T) {
	a := &Article{}
	if _, ok := a.CachedHTML(); ok {
		t.Fatalf("expected miss before StoreHTML")
	}
	a.StoreHTML("<p>hi</p>")
	if v, ok := a.CachedHTML(); !ok || v != "<p>hi</p>" {
		t.Errorf("CachedHTML after Store = %q ok=%v", v, ok)
	}
	a.StoreHTML("<p>changed</p>")
	v, _ := a.CachedHTML()
	if v != "<p>changed</p>" {
		t.Errorf("Store should overwrite, got %q", v)
	}
}

func TestAIBadgeSemantics(t *testing.T) {
	cases := []struct {
		raw       string
		wantIsAI  bool
		wantLabel string
	}{
		{"", false, ""},
		{"true", true, "AI"},
		{"True", true, "AI"},
		{"1", true, "AI"},
		{"yes", true, "AI"},
		{"on", true, "AI"},
		{"false", false, ""},   // explicit false isn't an AI badge
		{"claude", true, "claude"},
		{"GPT-4o", true, "GPT-4o"},
		{"gemini-2.5", true, "gemini-2.5"},
		{"  claude  ", true, "claude"}, // surrounding whitespace trimmed
	}
	for _, c := range cases {
		a := &Article{}
		a.AI = c.raw
		if got := a.IsAI(); got != c.wantIsAI {
			t.Errorf("IsAI(%q) = %v, want %v", c.raw, got, c.wantIsAI)
		}
		if c.wantIsAI {
			if got := a.AILabel(); got != c.wantLabel {
				t.Errorf("AILabel(%q) = %q, want %q", c.raw, got, c.wantLabel)
			}
		}
	}
}

func TestIsDraft(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"", false},
		{"true", true},
		{"True", true},
		{"1", true},
		{"yes", true},
		{"false", false},
		{"0", false},
		{"no", false},
	}
	for _, c := range cases {
		a := &Article{}
		a.Draft = c.raw
		if got := a.IsDraft(); got != c.want {
			t.Errorf("IsDraft(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Hello World", "hello-world"},
		{"A_B C.d", "a-b-c-d"},
		{"  spaces  ", "spaces"},
		{"--leading-and-trailing--", "leading-and-trailing"},
		{"中文", ""},               // CJK gets dropped
		{"中文 mixed 123", "mixed-123"},
		{"!@#$%", ""},
	}
	for _, c := range cases {
		if got := Slugify(c.in); got != c.want {
			t.Errorf("Slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSlugOrHashFallback(t *testing.T) {
	if got := SlugOrHash("中文"); got == "" {
		t.Errorf("expected hex fallback for CJK-only input, got empty")
	}
	if got := SlugOrHash("Hello"); got != "hello" {
		t.Errorf("SlugOrHash(Hello) = %q, want hello", got)
	}
}

func TestCoverFrontMatterIsParsed(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "with-cover.md")
	writeFile(t, p, `---
title: Hero
cover: hero.png
---
body
`)
	a, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if a.Cover != "hero.png" {
		t.Errorf("Cover = %q, want hero.png", a.Cover)
	}
}

func TestParseFileDoesNotRewrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "note.md")
	body := `---
title: Hello
tags: [a, b]
---
body
`
	writeFile(t, p, body)
	a, err := ParseFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// File on disk must still be byte-identical: ParseFile is read-only.
	got, _ := os.ReadFile(p)
	if string(got) != body {
		t.Errorf("ParseFile mutated source file:\n--got--\n%s\n--want--\n%s", got, body)
	}
	// YAML array tags still parse.
	if len(a.Tags) != 2 || a.Tags[0] != "a" || a.Tags[1] != "b" {
		t.Errorf("Tags = %v, want [a b]", a.Tags)
	}
}

func TestBodySummaryStripsMarkdownAndCodeBlocks(t *testing.T) {
	body := []byte(`# heading

> a quote

Plain text body here.

` + "```go\nshould be hidden\n```" + `

* list item
`)
	got := bodySummary(body, 200)
	if got == "" {
		t.Fatal("expected non-empty summary")
	}
	if containsAny(got, []string{"#", ">", "```", "should be hidden"}) {
		t.Errorf("summary leaked markup or code: %q", got)
	}
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}
