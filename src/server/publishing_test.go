package server

import (
	"github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/blog"
	"github.com/sbraveyoung/gobog/src/config"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCopyAssetsSkipsHiddenFiles(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	theme := t.TempDir()
	prev := config.C.Blog
	config.C.Blog.Source = root
	config.C.Blog.Theme = theme
	t.Cleanup(func() { config.C.Blog = prev })
	for _, name := range []string{"resource/image/photo.png", "resource/image/.DS_Store", "resource/image/.private/secret.png"} {
		p := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte("asset"), 0644)
	}
	os.WriteFile(filepath.Join(out, "index.html"), []byte(`<img src="/image/photo.png"><img src="/image/.DS_Store"><img src="/image/.private/secret.png">`), 0644)
	if err := copyAssets(out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "image/photo.png")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"image/.DS_Store", "image/.private/secret.png"} {
		if _, err := os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
			t.Errorf("hidden asset exported: %s", name)
		}
	}
}

func TestWithoutPrivateRecursivelyRemovesPrivateTitles(t *testing.T) {
	group := &article.Article{SubArticle: article.Articles{&article.Article{Meta: article.Meta{Title: "SECRET_TITLE", Private: "true"}}, &article.Article{Meta: article.Meta{Title: "public"}}}}
	got := withoutPrivate(article.Articles{group})
	if len(got) != 1 || len(got[0].SubArticle) != 1 {
		t.Fatalf("private title still in group: %+v", got[0].SubArticle)
	}
	if len(group.SubArticle) != 2 {
		t.Fatal("filter mutated shared blog tree")
	}
	if strings.Contains(got[0].SubArticle[0].Title, "SECRET") {
		t.Fatal("private title leaked")
	}
}

func TestCanonicalImagesRenderToExportedPaths(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "resource/image"), 0755)
	os.WriteFile(filepath.Join(root, "resource/image/photo.png"), []byte("image"), 0644)
	prev := config.C.Blog
	config.C.Blog.Source = root
	t.Cleanup(func() { config.C.Blog = prev })
	os.MkdirAll(filepath.Join(root, "post"), 0755)
	os.WriteFile(filepath.Join(root, "post/note.md"), []byte("---\ntitle: Test\nid: test\nurl: /post/test\ncreate_time: 2026-10-12 12:00:00\n---\nbody\n"), 0644)
	if err := blog.Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	for _, md := range []string{"![[photo.png]]", "![photo](../../image/photo.png)", "![photo](../resource/image/photo.png)"} {
		a := &article.Article{Source: filepath.Join(root, "post/note.md"), Content: []byte(md)}
		got, err := renderArticleHTML(a)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got, `src="/image/photo.png"`) {
			t.Errorf("%q points to wrong asset: %s", md, got)
		}
	}
}

func TestExportProvidesArchiveTagIndexAndThemeFeedLinks(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	prev := config.C.Blog
	config.C.Blog.Source = root
	config.C.Blog.Theme = "../../themes/minimal"
	t.Cleanup(func() { config.C.Blog = prev })
	os.MkdirAll(filepath.Join(root, "post"), 0755)
	os.WriteFile(filepath.Join(root, "post/hello.md"), []byte("---\ntitle: Hello\nurl: /post/hello\nid: hello\ncreate_time: 2026-10-12 12:00:00\ntags: [test]\n---\nbody\n"), 0644)
	if err := blog.Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := Export(out); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"archive/index.html", "tag/index.html"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing menu route %s", name)
		}
	}
	b, _ := os.ReadFile(filepath.Join(out, "__themes/sepia/index.html"))
	if strings.Contains(string(b), `href="/__themes/sepia/atom.xml"`) {
		t.Error("theme feed points to unexported feed")
	}
}

func TestExportAssetsIncludesOnlyPubliclyReferencedAttachments(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	theme := t.TempDir()
	prev := config.C.Blog
	config.C.Blog.Source = root
	config.C.Blog.Theme = theme
	t.Cleanup(func() { config.C.Blog = prev })
	for _, rel := range []string{"resource/image/public.png", "resource/image/draft-secret.png", "post/private.txt"} {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte("asset"), 0644)
	}
	os.WriteFile(filepath.Join(out, "index.html"), []byte(`<img src="/image/public.png">`), 0644)
	if err := copyAssets(out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(out, "image/public.png")); err != nil {
		t.Fatal("public image missing", err)
	}
	for _, rel := range []string{"image/draft-secret.png", "image/post/private.txt"} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Errorf("unpublished asset exported: %s", rel)
		}
	}
}

func TestHiddenFoldersLeaveNoRoutesFeedsTagsOrAttachmentsInAnyTheme(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	prev, export := config.C.Blog, config.ExportDir
	config.C.Blog.Source, config.C.Blog.Theme, config.C.Blog.Domain = root, "../../themes/minimal", "https://example.com"
	config.C.Blog.IncludeHidden, config.C.Blog.IncludeDrafts = false, false
	config.ExportDir = out
	t.Cleanup(func() { config.C.Blog = prev; config.ExportDir = export })
	notes := map[string]string{
		"post/public.md":         "---\ntitle: Public\nurl: /post/public\n---\n[[Hidden|未公开章节]]\n[**未公开章节**](/post/hidden#intro)\n[网页](https://example.com/about)\n[外站](https://elsewhere.example/post/hidden)\n",
		"post/secret/_folder.md": "---\nhidden: true\n---\n",
		"post/secret/note.md":    "---\ntitle: Hidden\nurl: /post/hidden\ntags: [hidden-tag]\n---\nSECRET_BODY ![[secret.png]]\n",
		"pages/_folder.md":       "---\nhidden: true\n---\n",
		"pages/about.md":         "---\ntitle: SECRET_ABOUT\nurl: /about\n---\nSECRET_BODY\n",
	}
	for rel, s := range notes {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(s), 0644)
	}
	os.MkdirAll(filepath.Join(root, "resource/image"), 0755)
	os.WriteFile(filepath.Join(root, "resource/image/secret.png"), []byte("secret"), 0644)
	if err := blog.Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	if err := Export(out); err != nil {
		t.Fatal(err)
	}
	err := filepath.Walk(out, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, "secret.png") {
			t.Errorf("hidden-only attachment exported: %s", p)
		}
		if strings.HasSuffix(p, ".html") || strings.HasSuffix(p, ".xml") {
			data, _ := os.ReadFile(p)
			if regexp.MustCompile(`href="(?:/__themes/[^/]+)?/(?:about|post/hidden)(?:["/#?])`).Match(data) {
				t.Errorf("hidden route survives theme rewriting: %s", p)
			}
			for _, s := range []string{"SECRET_BODY", "SECRET_ABOUT", "hidden-tag", `href="/about"`, `href="/post/hidden`, `href="https://example.com/about`, "<loc>https://example.com/about</loc>"} {
				if s == "/post/hidden" && strings.Contains(string(data), "https://elsewhere.example/post/hidden") {
					continue
				}
				if strings.Contains(string(data), s) {
					t.Errorf("hidden data %s leaked in %s", s, p)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestExportRefusesNonemptyOutputToPreventStaleHiddenPages(t *testing.T) {
	out := t.TempDir()
	os.WriteFile(filepath.Join(out, "old-private.html"), []byte("old content"), 0644)
	if err := Export(out); err == nil {
		t.Fatal("reuse of a stale output directory accepted")
	}
}

func TestSitemapListsOnlyActualPublicPagesAndUsesUpdatedDate(t *testing.T) {
	prev := config.C.Blog
	config.C.Blog.Domain = "https://example.com"
	t.Cleanup(func() { config.C.Blog = prev })
	page := &article.Article{Meta: article.Meta{Title: "Contact", URL: "/contact", CreateTime: "2018-01-01 00:00:00", UpdatedTime: "2026-10-12 12:00:00"}}
	blog.Blog.SetForTesting(map[string]article.Articles{blog.BlogTypes["page"]: {page}})
	sm := buildSitemap()
	found := false
	for _, u := range sm.URLs {
		if u.Loc == "https://example.com/about" {
			t.Fatal("hardcoded about route leaked")
		}
		if u.Loc == "https://example.com/contact" {
			found = true
			if u.LastMod != "2026-10-12" {
				t.Fatal("updated_time ignored")
			}
		}
	}
	if !found {
		t.Fatal("public independent page missing from sitemap")
	}
}

func TestCopyAssetsNeverExportsMarkdownOrHiddenFolderFiles(t *testing.T) {
	root := t.TempDir()
	out := t.TempDir()
	theme := t.TempDir()
	prev := config.C.Blog
	config.C.Blog.Source, config.C.Blog.Theme, config.C.Blog.IncludeHidden = root, theme, false
	t.Cleanup(func() { config.C.Blog = prev })
	for rel, body := range map[string]string{"post/secret/note.md": "SECRET_BODY", "post/secret/image.png": "secret", "post/secret/_folder.md": "---\nhidden: true\n---\n"} {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(body), 0644)
	}
	os.WriteFile(filepath.Join(out, "index.html"), []byte(`<a href="/image/post/secret/note.md">source</a><img src="/image/post/secret/image.png">`), 0644)
	if err := copyAssets(out); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"image/post/secret/note.md", "image/post/secret/image.png"} {
		if _, err := os.Stat(filepath.Join(out, rel)); !os.IsNotExist(err) {
			t.Errorf("withheld source or asset exported: %s", rel)
		}
	}
}

func TestWithheldMarkdownLinksKeepLabelsButTyposAndExternalLinksRemain(t *testing.T) {
	root := t.TempDir()
	prev := config.C.Blog
	config.C.Blog.Source, config.C.Blog.Domain, config.C.Blog.IncludeHidden, config.C.Blog.IncludeDrafts = root, "https://example.com", false, false
	t.Cleanup(func() { config.C.Blog = prev })
	for rel, body := range map[string]string{
		"post/public.md": "---\ntitle: Public\nurl: /post/public\n---\n[**草稿**](/post/hidden#intro) [同站](https://example.com/post/hidden) [外站](https://elsewhere.example/post/hidden) [拼错](/post/misspelled)",
		"post/hidden.md": "---\ntitle: Hidden\nurl: /post/hidden\ndraft: true\n---\nbody",
	} {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0755)
		os.WriteFile(p, []byte(body), 0644)
	}
	if err := blog.Blog.Reload(); err != nil {
		t.Fatal(err)
	}
	html, err := renderArticleHTML(blog.Blog.FindByURL("/post/public"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(html, `href="/post/hidden`) || strings.Contains(html, `href="https://example.com/post/hidden`) {
		t.Fatal("withheld article link remains:", html)
	}
	for _, s := range []string{"<strong>草稿</strong>", `href="https://elsewhere.example/post/hidden"`, `href="/post/misspelled"`} {
		if !strings.Contains(html, s) {
			t.Errorf("label or unmodified link %s lost: %s", s, html)
		}
	}
}
