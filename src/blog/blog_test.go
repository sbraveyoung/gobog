package blog

import (
	"os"
	"path/filepath"
	"testing"

	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/SmartBrave/gobog/src/config"
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

// TestReloadVaultLayout points the blog at an Obsidian-style folder (no
// "post/" subdir) and verifies arbitrary depth + path-derived URLs +
// about/ separation + image indexing all work.
func TestReloadVaultLayout(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "Hello.md"), "---\ntitle: Hello\ncreate_time: 2026-04-25 09:00:00\n---\nbody\n")
	writeNote(t, filepath.Join(root, "Tech", "HTTP.md"), "---\ntitle: HTTP\ncreate_time: 2026-04-24 09:00:00\n---\nseebar [[Hello]]\n")
	writeNote(t, filepath.Join(root, "Tech", "Networking", "TLS.md"), "---\ntitle: TLS\ncreate_time: 2026-04-23 09:00:00\n---\ntls\n")
	writeNote(t, filepath.Join(root, "about", "me.md"), "---\ntitle: About Me\n---\nme\n")
	writeNote(t, filepath.Join(root, "Tech", "Networking", "diagram.png"), "fakepng")
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

	abouts := Blog.Articles(BlogTypes["about"])
	if len(abouts) != 1 || abouts[0].URL != "/about" {
		t.Errorf("about: got %v", abouts)
	}

	wiki := Blog.Wiki()
	if got := wiki.ResolveNote("Hello"); got != "/post/hello" {
		t.Errorf("ResolveNote(Hello) = %q, want /post/hello", got)
	}
	if got := wiki.ResolveNote("Tech/HTTP"); got != "/post/tech/http" {
		t.Errorf("ResolveNote(Tech/HTTP) = %q, want /post/tech/http", got)
	}
	if got := wiki.ResolveImage("diagram.png"); got != "Tech/Networking/diagram.png" {
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

// TestReloadLegacyLayoutStillWorks covers back-compat: when <source>/post/
// exists, the scanner uses the original gobog convention (1 level of group
// nesting + crc32 hex IDs) instead of the vault layout.
func TestReloadLegacyLayoutStillWorks(t *testing.T) {
	root := t.TempDir()
	writeNote(t, filepath.Join(root, "post", "leaf.md"), "")
	writeNote(t, filepath.Join(root, "post", "group", "sub.md"), "")

	prev := config.C.Blog.Source
	config.C.Blog.Source = root
	t.Cleanup(func() { config.C.Blog.Source = prev })

	if err := Blog.Reload(); err != nil {
		t.Fatal(err)
	}

	groups := Blog.Groups()
	if len(groups) != 2 {
		t.Fatalf("legacy layout: want 2 top-level entries, got %d", len(groups))
	}
	hasGroup := false
	for _, g := range groups {
		if len(g.SubArticle) > 0 {
			hasGroup = true
		}
	}
	if !hasGroup {
		t.Error("expected at least one group with sub-articles in legacy layout")
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
