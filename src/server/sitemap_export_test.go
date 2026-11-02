package server

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sbraveyoung/gobog/src/blog"
	"github.com/sbraveyoung/gobog/src/config"
)

func TestExportSitemapIncludesPublicSeriesLandingPages(t *testing.T) {
	source, out := t.TempDir(), t.TempDir()
	previousBlog, previousExport := blog.Blog, config.ExportDir
	previousConfig := config.C.Blog
	blog.Blog = &blog.BlogST{}
	config.C.Blog = config.BlogConfig{
		Source: source, Theme: "../../themes/minimal", Domain: "https://example.com",
	}
	config.ExportDir = out
	t.Cleanup(func() {
		blog.Blog, config.ExportDir = previousBlog, previousExport
		config.C.Blog = previousConfig
	})

	notes := map[string]string{
		"post/series/one.md":        "---\ntitle: One\nurl: /post/series/one\ncreate_time: 2018-01-01 00:00:00\nupdated_time: 2026-10-12 12:00:00\n---\nFirst chapter\n",
		"post/series/nested/two.md": "---\ntitle: Two\nurl: /post/series/nested/two\ncreate_time: 2020-02-03 00:00:00\n---\nNested chapter\n",
		"post/series/private.md":    "---\ntitle: Private\nprivate: true\n---\nPrivate chapter\n",
		"post/private-only/note.md": "---\ntitle: Private only\nprivate: true\n---\nPrivate body\n",
		"post/draft-only/note.md":   "---\ntitle: Draft\ndraft: true\n---\nDraft body\n",
		"post/hidden/_folder.md":    "---\nhidden: true\n---\n",
		"post/hidden/note.md":       "---\ntitle: Hidden\n---\nHidden body\n",
		"pages/contact.md":          "---\ntitle: Contact\nurl: /contact\ncreate_time: 2019-01-01 00:00:00\nupdated_time: 2026-10-10 00:00:00\n---\nContact page\n",
	}
	for rel, body := range notes {
		path := filepath.Join(source, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := blog.Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := Export(out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "sitemap.xml"))
	if err != nil {
		t.Fatal(err)
	}
	var sitemap sitemapSet
	if err := xml.Unmarshal(data, &sitemap); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"/":                       "",
		"/post/series":            "",
		"/post/series/nested":     "",
		"/post/series/one":        "2026-10-12",
		"/post/series/nested/two": "2020-02-03",
		"/contact":                "2026-10-10",
	}
	for _, entry := range sitemap.URLs {
		route := strings.TrimPrefix(entry.Loc, "https://example.com")
		lastmod, ok := want[route]
		if !ok {
			t.Errorf("unexpected or duplicate sitemap URL: %s", entry.Loc)
			continue
		}
		if entry.LastMod != lastmod {
			t.Errorf("%s lastmod = %q, want %q", route, entry.LastMod, lastmod)
		}
		delete(want, route)
	}
	for route := range want {
		t.Errorf("public route missing from sitemap: %s", route)
	}
	for _, path := range []string{"post/series/index.html", "post/series/nested/index.html"} {
		if _, err := os.Stat(filepath.Join(out, path)); err != nil {
			t.Errorf("series landing page missing: %s: %v", path, err)
		}
	}
	for _, path := range []string{"post/series/private", "post/private-only", "post/draft-only", "post/hidden"} {
		if _, err := os.Stat(filepath.Join(out, path)); !os.IsNotExist(err) {
			t.Errorf("unpublished route exported: %s (stat error: %v)", path, err)
		}
	}
}
