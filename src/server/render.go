package server

import (
	"bytes"
	"fmt"

	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var mdRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		html.WithXHTML(),
		html.WithUnsafe(),
	),
)

// renderArticleHTML returns the cached HTML for an article, rendering it on
// demand the first time. The result is stored on the article for subsequent
// reads.
func renderArticleHTML(article *articlepkg.Article) (string, error) {
	if cached, ok := article.CachedHTML(); ok {
		return cached, nil
	}
	src := append([]byte("## "+article.Title+"\n"), article.Content...)
	var buf bytes.Buffer
	if err := mdRenderer.Convert(src, &buf); err != nil {
		return "", fmt.Errorf("render %q: %w", article.URL, err)
	}
	html := buf.String()
	article.StoreHTML(html)
	return html, nil
}

// articleView wraps an article for template rendering. Embedding lets templates
// keep using {{.Title}}, {{.URL}}, etc. while we shadow Parse with a
// per-request value (avoids racing on Article.Parse across goroutines).
type articleView struct {
	*articlepkg.Article
	Parse       string
	Domain      string
	Canonical   string
	SiteTitle   string
	Description string
}

func newArticleView(a *articlepkg.Article, parse, domain string) articleView {
	desc := a.Summary
	if desc == "" {
		desc = a.Description
	}
	return articleView{
		Article:     a,
		Parse:       parse,
		Domain:      domain,
		Canonical:   domain + a.URL,
		SiteTitle:   a.Title,
		Description: desc,
	}
}
