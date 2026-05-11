package blog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	articlepkg "github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/config"
)

func writeNote(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReloadVaultLayout points the blog at an Obsidian-style folder with the
// canonical post/ + pages/ + resource/image/ split and verifies arbitrary
// depth + path-derived URLs + page separation + image indexing all work.
func TestReloadVaultLayout(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "post", "Hello.md"), "---\ntitle: Hello\ncreate_time: 2026-04-25 09:00:00\n---\nbody\n")
	writeNote(t, filepath.Join(root, "post", "Tech", "HTTP.md"), "---\ntitle: HTTP\ncreate_time: 2026-04-24 09:00:00\n---\nseebar [[Hello]]\n")
	writeNote(t, filepath.Join(root, "post", "Tech", "Networking", "TLS.md"), "---\ntitle: TLS\ncreate_time: 2026-04-23 09:00:00\n---\ntls\n")
	writeNote(t, filepath.Join(root, "pages", "about.md"), "---\ntitle: About\n---\nme\n")
	writeNote(t, filepath.Join(root, "pages", "contact.md"), "---\ntitle: Contact\n---\ncontact\n")
	writeNote(t, filepath.Join(root, "resource", "image", "diagram.png"), "fakepng")
	writeNote(t, filepath.Join(root, ".obsidian", "config"), "ignore me")

	prev := config.C.Blog.Source
	config.C.Blog.Source = root
	t.Cleanup(func() { config.C.Blog.Source = prev })

	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}

	posts := Blog.AllPosts()
	urls := map[string]bool{}
	for _, a := range posts {
		urls[a.URL] = true
	}
	wants := []string{"/post/hello", "/post/tech/http", "/post/tech/networking/tls"}
	for _, u := range wants {
		if !urls[u] {
			t.Errorf("missing post URL %q in %v", u, urls)
		}
	}

	pages := Blog.Articles(BlogTypes["page"])
	pageURLs := map[string]bool{}
	for _, p := range pages {
		pageURLs[p.URL] = true
	}
	if !pageURLs["/about"] || !pageURLs["/contact"] {
		t.Errorf("pages: want /about and /contact, got %v", pageURLs)
	}

	wiki := Blog.Wiki()
	if got := wiki.ResolveNote("Hello"); got != "/post/hello" {
		t.Errorf("ResolveNote(Hello) = %q, want /post/hello", got)
	}
	if got := wiki.ResolveNote("Tech/HTTP"); got != "/post/tech/http" {
		t.Errorf("ResolveNote(Tech/HTTP) = %q, want /post/tech/http", got)
	}
	if got := wiki.ResolveImage("diagram.png"); got != "resource/image/diagram.png" {
		t.Errorf("ResolveImage(diagram.png) = %q", got)
	}
	if got := wiki.ResolveImage("missing.png"); got != "" {
		t.Errorf("ResolveImage(missing) should be empty, got %q", got)
	}

	// Hidden directories (.obsidian) must not surface as posts.
	for u := range urls {
		if filepath.Base(u) == "config" {
			t.Errorf(".obsidian content leaked into posts: %v", urls)
		}
	}
}

// TestReloadPersistsMissingMeta covers the original gobog product design:
// when an article is missing id / url / title / create_time, the scanner
// fills them in deterministically and writes the result back to disk, so
// the URL stays stable even if the file is later renamed or moved.
func TestReloadPersistsMissingMeta(t *testing.T) {
	root := t.TempDir()
	notePath := filepath.Join(root, "post", "Tech", "HTTP.md")
	writeNote(t, notePath, "no front matter at all\n")

	prev := config.C.Blog.Source
	config.C.Blog.Source = root
	t.Cleanup(func() { config.C.Blog.Source = prev })

	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), "---") {
		t.Errorf("scanner did not persist front-matter back; file body:\n%s", got)
	}

	// Reparse — id / url / title / create_time must all be populated.
	a, err := articlepkg.ParseFile(notePath)
	if err != nil {
		t.Fatal(err)
	}
	if a.Id == "" || a.URL == "" || a.Title == "" || a.CreateTime == "" {
		t.Errorf("expected all defaults persisted, got %+v", a.Meta)
	}
}

// TestExcludeDirs verifies [blog].exclude_dirs keeps the named top-level
// folders inside <source>/post/ out of the listing.
func TestExcludeDirs(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "post", "Hello.md"), "")
	writeNote(t, filepath.Join(root, "post", "Templates", "tpl.md"), "")
	writeNote(t, filepath.Join(root, "post", "Drafts", "wip.md"), "")
	writeNote(t, filepath.Join(root, "post", "Tech", "ok.md"), "")

	prevSrc, prevExcl := config.C.Blog.Source, config.C.Blog.ExcludeDirs
	config.C.Blog.Source = root
	config.C.Blog.ExcludeDirs = []string{"Templates", "drafts"} // mixed case
	t.Cleanup(func() {
		config.C.Blog.Source = prevSrc
		config.C.Blog.ExcludeDirs = prevExcl
	})

	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	urls := map[string]bool{}
	for _, a := range Blog.AllPosts() {
		urls[a.URL] = true
	}
	for _, want := range []string{"/post/hello", "/post/tech/ok"} {
		if !urls[want] {
			t.Errorf("missing %q in %v", want, urls)
		}
	}
	for url := range urls {
		if strings.HasPrefix(url, "/post/templates") || strings.HasPrefix(url, "/post/drafts") {
			t.Errorf("excluded dir leaked into listing: %s", url)
		}
	}
}

// TestLayoutLegacyURLs verifies [blog].layout = "legacy" generates URLs
// matching the original gobog scheme — `/post/<crc32(parent)>/<crc32(body)>`
// for nested files, `/post/<crc32(body)>` for files at <source>/post/. The
// scanner shape (recursive walk) is the same regardless of layout.
func TestLayoutLegacyURLs(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "post", "Hello.md"), "hello body\n")
	writeNote(t, filepath.Join(root, "post", "Tech", "http.md"), "http body\n")

	prevSrc, prevLayout := config.C.Blog.Source, config.C.Blog.Layout
	config.C.Blog.Source = root
	config.C.Blog.Layout = "legacy"
	t.Cleanup(func() {
		config.C.Blog.Source = prevSrc
		config.C.Blog.Layout = prevLayout
	})

	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}

	hexRE := regexp.MustCompile(`^/post/[0-9a-f]+(/[0-9a-f]+)?$`)
	for _, a := range Blog.AllPosts() {
		if !hexRE.MatchString(a.URL) {
			t.Errorf("layout=legacy URL not in /post/<hex>(/<hex>) form: %s", a.URL)
		}
	}
}

// TestLayoutVaultURLs (default) confirms slugified path URLs from inside
// the canonical <source>/post/ tree.
func TestLayoutVaultURLs(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "post", "in-post.md"), "")
	writeNote(t, filepath.Join(root, "post", "Hello.md"), "")
	writeNote(t, filepath.Join(root, "post", "Tech", "http.md"), "")

	prevSrc, prevLayout := config.C.Blog.Source, config.C.Blog.Layout
	config.C.Blog.Source = root
	config.C.Blog.Layout = "vault"
	t.Cleanup(func() {
		config.C.Blog.Source = prevSrc
		config.C.Blog.Layout = prevLayout
	})

	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	urls := map[string]bool{}
	for _, a := range Blog.AllPosts() {
		urls[a.URL] = true
	}
	for _, want := range []string{"/post/in-post", "/post/hello", "/post/tech/http"} {
		if !urls[want] {
			t.Errorf("layout=vault: missing %q in %v", want, urls)
		}
	}
}

// Ensure the slug helper is wired the way the scanner expects.
func TestSlugifyPath(t *testing.T) {
	if got := slugifyPath("Tech/Networking/HTTP"); got != "tech/networking/http" {
		t.Errorf("slugifyPath = %q, want tech/networking/http", got)
	}
	// Empty per-segment slugs (CJK names) fall back to a hash so the path
	// stays addressable instead of producing a literal "//".
	got := slugifyPath("中文/笔记")
	if got == "" {
		t.Errorf("expected non-empty path, got empty")
	}
	for _, part := range []string{"", "/"} {
		if got == part {
			t.Errorf("slugifyPath produced %q for CJK input — expected hex segments", got)
		}
	}
	// Force articlepkg to be used so go vet doesn't complain.
	_ = articlepkg.TIME_LAYOUT
}

// TestSeriesGroupSortsAscending covers the per-group sequential
// ordering: when every leaf in a directory carries an order signal
// (front-matter `order:` or a digit-prefix filename), the group's
// SubArticle list should run 1 → N instead of newest-first.
//
// Mirrors the AI-入门到精通系列 use case from the blog: files named
// 00-, 01-, 02-, … must read in that order, not date-desc.
func TestSeriesGroupSortsAscending(t *testing.T) {
	root := t.TempDir()
	// Intentionally create the files newest-first so the date-desc
	// default would order them backwards (10, 02, 00) — series
	// detection must override that.
	writeNote(t, filepath.Join(root, "post", "AI系列", "10-finale.md"),
		"---\ntitle: finale\ncreate_time: 2026-05-10 10:00:00\n---\nbody\n")
	writeNote(t, filepath.Join(root, "post", "AI系列", "02-middle.md"),
		"---\ntitle: middle\ncreate_time: 2026-05-08 10:00:00\n---\nbody\n")
	writeNote(t, filepath.Join(root, "post", "AI系列", "00-intro.md"),
		"---\ntitle: intro\ncreate_time: 2026-05-01 10:00:00\n---\nbody\n")

	prev := config.C.Blog.Source
	config.C.Blog.Source = root
	t.Cleanup(func() { config.C.Blog.Source = prev })

	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}

	var group *articlepkg.Article
	for _, g := range Blog.Groups() {
		if g.Title == "AI系列" {
			group = g
			break
		}
	}
	if group == nil {
		t.Fatal("AI系列 group not found in Blog.Groups()")
	}
	titles := []string{}
	for _, a := range group.SubArticle {
		titles = append(titles, a.Title)
	}
	want := []string{"intro", "middle", "finale"}
	for i, w := range want {
		if titles[i] != w {
			t.Errorf("series order: want %v, got %v", want, titles)
			break
		}
	}

	// Group's own CreateTime should still be the newest sub's date so
	// the group floats in the home listing when a fresh chapter lands.
	if group.CreateTime != "2026-05-10 10:00:00" {
		t.Errorf("group CreateTime = %q, want 2026-05-10 (newest sub)", group.CreateTime)
	}
}

// TestNonSeriesGroupKeepsDateOrder confirms the default ordering still
// applies when leaves don't carry a series signal (most blog groups).
func TestNonSeriesGroupKeepsDateOrder(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "post", "杂记", "alpha.md"),
		"---\ntitle: alpha\ncreate_time: 2026-05-01 10:00:00\n---\nbody\n")
	writeNote(t, filepath.Join(root, "post", "杂记", "beta.md"),
		"---\ntitle: beta\ncreate_time: 2026-05-09 10:00:00\n---\nbody\n")

	prev := config.C.Blog.Source
	config.C.Blog.Source = root
	t.Cleanup(func() { config.C.Blog.Source = prev })

	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}

	var group *articlepkg.Article
	for _, g := range Blog.Groups() {
		if g.Title == "杂记" {
			group = g
			break
		}
	}
	if group == nil {
		t.Fatal("杂记 group not found")
	}
	if group.SubArticle[0].Title != "beta" {
		t.Errorf("non-series group should sort date-desc; got %s first",
			group.SubArticle[0].Title)
	}
}
