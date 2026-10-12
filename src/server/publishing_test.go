package server

import (
	"github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/blog"
	"github.com/sbraveyoung/gobog/src/config"
	"os"
	"path/filepath"
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
