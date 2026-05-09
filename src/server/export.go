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

	articlepkg "github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/blog"
	"github.com/sbraveyoung/gobog/src/config"
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
	if err := t.Execute(&buf, withoutPrivate(blog.Blog.Groups())); err != nil {
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
	groupTpl, err := template.ParseFiles(config.C.Blog.Theme + "/index.html")
	if err != nil {
		return err
	}
	var walk func(articlepkg.Articles) error
	walk = func(list articlepkg.Articles) error {
		for _, a := range list {
			// Private articles can't enforce auth on a static host, so they
			// drop out of the export entirely. Their URL keeps resolving in
			// server mode (where the auth gate works).
			if a.IsPrivate() {
				continue
			}
			if len(a.SubArticle) == 0 {
				if err := exportLeaf(outDir, a); err != nil {
					return err
				}
				continue
			}
			// Group landing: feed the template a list with private subs
			// stripped so the rendered page never links to a 404. If the
			// filter empties the group entirely, skip rendering — the
			// parent already dropped the link via withoutPrivate.
			subs := withoutPrivate(a.SubArticle)
			if len(subs) == 0 {
				continue
			}
			var buf bytes.Buffer
			if err := groupTpl.Execute(&buf, subs); err != nil {
				return err
			}
			if err := writeFile(urlToFile(outDir, a.URL), buf.Bytes()); err != nil {
				return err
			}
			if err := walk(a.SubArticle); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(blog.Blog.Groups())
}

// withoutPrivate returns a shallow copy of list with private articles
// removed, recursing into groups so a group whose subs are all private
// drops out entirely. Used by export so the rendered tree never contains a
// link to a path that the export skipped.
func withoutPrivate(list articlepkg.Articles) articlepkg.Articles {
	out := make(articlepkg.Articles, 0, len(list))
	for _, a := range list {
		if a.IsPrivate() {
			continue
		}
		if len(a.SubArticle) > 0 {
			subs := withoutPrivate(a.SubArticle)
			if len(subs) == 0 {
				// Empty group after filtering — skip altogether so the
				// parent listing doesn't link to a 404.
				continue
			}
		}
		out = append(out, a)
	}
	return out
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
		filtered := withoutPrivate(blog.Blog.PostsByTag(tag))
		if len(filtered) == 0 {
			continue
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, filtered); err != nil {
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
	// Theme assets (CSS/JS): straight directory copy.
	for _, sub := range []string{"css", "js"} {
		src := filepath.Join(config.C.Blog.Theme, sub)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return err
		}
		if err := copyTree(src, filepath.Join(outDir, sub)); err != nil {
			return fmt.Errorf("copy %s: %w", src, err)
		}
	}

	// Legacy <source>/image/ keeps working as before.
	legacyImg := filepath.Join(config.C.Blog.Source, "image")
	if fi, err := os.Stat(legacyImg); err == nil && fi.IsDir() {
		if err := copyTree(legacyImg, filepath.Join(outDir, "image")); err != nil {
			return fmt.Errorf("copy %s: %w", legacyImg, err)
		}
	}

	// Vault mode: every non-.md file referenced by the image index gets
	// copied to <outDir>/image/<rel-path>, preserving its layout so URLs
	// like /image/Tech/Networking/diagram.png stay valid offline.
	source := config.C.Blog.Source
	return filepath.Walk(source, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && p != source {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
			return nil
		}
		rel, err := filepath.Rel(source, p)
		if err != nil {
			return nil
		}
		// Don't double-copy the legacy <source>/image/ tree.
		if strings.HasPrefix(filepath.ToSlash(rel), "image/") {
			return nil
		}
		return copyFile(p, filepath.Join(outDir, "image", rel))
	})
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
