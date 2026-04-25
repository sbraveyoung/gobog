package server

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"net/http/httptest"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	ttemplate "text/template"

	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/SmartBrave/gobog/src/blog"
	"github.com/SmartBrave/gobog/src/config"
	"github.com/astaxie/beego/logs"
)

// Export renders the entire site as static HTML/XML into outDir. Layout matches
// the gobog HTTP routes one-for-one so the result is drop-in compatible with
// GitHub Pages repos like sbraveyoung/sbraveyoung.github.io:
//
//	outDir/
//	├── index.html
//	├── about/index.html
//	├── post/<id>/index.html              (leaf post)
//	├── post/<id>/index.html              (group landing)
//	├── post/<id>/<sub>/index.html        (sub post)
//	├── tag/<tag>/index.html
//	├── 404.html
//	├── atom.xml
//	├── sitemap.xml
//	├── robots.txt
//	├── CNAME                            (if [blog].cname is set)
//	├── css/...                          (copied from theme)
//	├── js/...                           (copied from theme)
//	└── image/...                        (copied from blog source/image)
func Export(outDir string) error {
	if outDir == "" {
		return fmt.Errorf("export dir is empty")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", outDir, err)
	}

	if err := exportIndex(outDir); err != nil {
		return err
	}
	if err := exportAbout(outDir); err != nil {
		return err
	}
	if err := exportPosts(outDir); err != nil {
		return err
	}
	if err := exportTags(outDir); err != nil {
		return err
	}
	if err := exportFeeds(outDir); err != nil {
		return err
	}
	if err := exportNotFound(outDir); err != nil {
		return err
	}
	if err := copyAssets(outDir); err != nil {
		return err
	}
	if err := writeCNAME(outDir); err != nil {
		return err
	}

	logs.Info("export complete:", outDir)
	return nil
}

func exportIndex(outDir string) error {
	t, err := template.ParseFiles(config.C.Blog.Theme + "/index.html")
	if err != nil {
		return fmt.Errorf("parse index template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, blog.Blog.Groups()); err != nil {
		return fmt.Errorf("exec index template: %w", err)
	}
	return writeFile(filepath.Join(outDir, "index.html"), buf.Bytes())
}

func exportAbout(outDir string) error {
	list := blog.Blog.Articles(blog.BlogTypes["about"])
	if len(list) == 0 {
		return nil
	}
	html, err := renderArticleHTML(list[0])
	if err != nil {
		return err
	}
	body, err := executePostTemplate(list[0], html)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "about", "index.html"), body)
}

func exportPosts(outDir string) error {
	for _, top := range blog.Blog.Groups() {
		if len(top.SubArticle) == 0 {
			if err := exportLeaf(outDir, top); err != nil {
				return err
			}
			continue
		}
		// Group landing: render the group's sub-list using index.html so the
		// inner page mirrors what postHandler returns for the group URL.
		t, err := template.ParseFiles(config.C.Blog.Theme + "/index.html")
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, top.SubArticle); err != nil {
			return err
		}
		if err := writeFile(urlToFile(outDir, top.URL), buf.Bytes()); err != nil {
			return err
		}
		for _, sub := range top.SubArticle {
			if err := exportLeaf(outDir, sub); err != nil {
				return err
			}
		}
	}
	return nil
}

func exportLeaf(outDir string, a *articlepkg.Article) error {
	html, err := renderArticleHTML(a)
	if err != nil {
		return err
	}
	body, err := executePostTemplate(a, html)
	if err != nil {
		return err
	}
	return writeFile(urlToFile(outDir, a.URL), body)
}

func exportTags(outDir string) error {
	tags := blog.Blog.Tags()
	if len(tags) == 0 {
		return nil
	}
	t, err := template.ParseFiles(config.C.Blog.Theme + "/index.html")
	if err != nil {
		return err
	}
	for _, tag := range tags {
		var buf bytes.Buffer
		if err := t.Execute(&buf, blog.Blog.PostsByTag(tag)); err != nil {
			return err
		}
		if err := writeFile(filepath.Join(outDir, "tag", tag, "index.html"), buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func exportFeeds(outDir string) error {
	feed := buildAtomFeed()
	var fbuf bytes.Buffer
	fbuf.WriteString(xml.Header)
	enc := xml.NewEncoder(&fbuf)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(outDir, "atom.xml"), fbuf.Bytes()); err != nil {
		return err
	}

	sm := buildSitemap()
	var sbuf bytes.Buffer
	sbuf.WriteString(xml.Header)
	enc2 := xml.NewEncoder(&sbuf)
	enc2.Indent("", "  ")
	if err := enc2.Encode(sm); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(outDir, "sitemap.xml"), sbuf.Bytes()); err != nil {
		return err
	}

	domain := strings.TrimRight(config.C.Blog.Domain, "/")
	robots := fmt.Sprintf("User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", domain)
	return writeFile(filepath.Join(outDir, "robots.txt"), []byte(robots))
}

func exportNotFound(outDir string) error {
	rec := httptest.NewRecorder()
	notFound(rec, httptest.NewRequest("GET", "/__missing__", nil))
	return writeFile(filepath.Join(outDir, "404.html"), rec.Body.Bytes())
}

func executePostTemplate(a *articlepkg.Article, html string) ([]byte, error) {
	t, err := ttemplate.ParseFiles(config.C.Blog.Theme + "/post.html")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	view := newArticleView(a, html, config.C.Blog.Domain)
	if err := t.Execute(&buf, view); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// urlToFile maps a route like /post/abc or /post/abc/def to outDir/post/abc/index.html.
func urlToFile(outDir, url string) string {
	clean := strings.TrimSuffix(strings.TrimPrefix(url, "/"), "/")
	if clean == "" {
		return filepath.Join(outDir, "index.html")
	}
	return filepath.Join(outDir, filepath.FromSlash(clean), "index.html")
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func copyAssets(outDir string) error {
	pairs := []struct{ src, dst string }{
		{filepath.Join(config.C.Blog.Theme, "css"), filepath.Join(outDir, "css")},
		{filepath.Join(config.C.Blog.Theme, "js"), filepath.Join(outDir, "js")},
		{filepath.Join(config.C.Blog.Source, "image"), filepath.Join(outDir, "image")},
	}
	for _, p := range pairs {
		if _, err := os.Stat(p.src); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := copyTree(p.src, p.dst); err != nil {
			return fmt.Errorf("copy %s: %w", p.src, err)
		}
	}
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(p, target)
	})
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func writeCNAME(outDir string) error {
	cname := strings.TrimSpace(config.C.Blog.CNAME)
	if cname == "" {
		return nil
	}
	return writeFile(filepath.Join(outDir, "CNAME"), []byte(cname+"\n"))
}

// keep pathpkg used; small helper to validate URLs from the article model.
var _ = pathpkg.Join
