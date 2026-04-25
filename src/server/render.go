package server

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/SmartBrave/gobog/src/blog"
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

// Wikilink syntax (Obsidian):
//   [[Page Title]]                  → link to a note titled "Page Title"
//   [[Page Title|display text]]     → link with custom anchor text
//   [[Page Title#section]]          → fragment after the path is appended
//   ![[image.png]]                  → embed an image
//   ![[note]]                       → embed of another note (rendered as link;
//                                     full inlining is intentionally out of scope)
//
// We pre-process the markdown text before handing it to goldmark so this
// stays decoupled from goldmark internals. Order matters: ![[ must be matched
// before [[ to avoid eating the inner brackets.
var (
	embedRE    = regexp.MustCompile(`!\[\[([^\]\n]+)\]\]`)
	wikilinkRE = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)
)

// renderArticleHTML returns the cached HTML for an article, rendering it on
// demand the first time. Wikilinks and image embeds are expanded against the
// blog's wiki index.
func renderArticleHTML(article *articlepkg.Article) (string, error) {
	if cached, ok := article.CachedHTML(); ok {
		return cached, nil
	}
	src := append([]byte("## "+article.Title+"\n"), article.Content...)
	src = expandWikilinks(src, blog.Blog.Wiki())

	var buf bytes.Buffer
	if err := mdRenderer.Convert(src, &buf); err != nil {
		return "", fmt.Errorf("render %q: %w", article.URL, err)
	}
	out := buf.String()
	article.StoreHTML(out)
	return out, nil
}

// expandWikilinks rewrites Obsidian-style [[...]] / ![[...]] tokens into
// vanilla markdown links and HTML <img> tags using the wiki index. Unresolved
// wikilinks are left as plain text (display text if provided, else the raw
// target) so a missing note doesn't break rendering.
func expandWikilinks(src []byte, wiki *blog.WikiIndex) []byte {
	src = embedRE.ReplaceAllFunc(src, func(m []byte) []byte {
		inner := string(embedRE.FindSubmatch(m)[1])
		target, alt := splitWikilink(inner)
		if isImageRef(target) {
			path := wiki.ResolveImage(filepath_Base(target))
			if path == "" {
				return []byte(htmlEscape(alt))
			}
			return []byte(fmt.Sprintf(`<img src="/image/%s" alt=%q>`, path, alt))
		}
		// Note embeds aren't fully inlined; render as a link instead.
		if u := wiki.ResolveNote(target); u != "" {
			return []byte(fmt.Sprintf("[%s](%s)", escapeMD(alt), u))
		}
		return []byte(escapeMD(alt))
	})

	src = wikilinkRE.ReplaceAllFunc(src, func(m []byte) []byte {
		inner := string(wikilinkRE.FindSubmatch(m)[1])
		target, display := splitWikilink(inner)
		base, frag := splitFragment(target)
		if u := wiki.ResolveNote(base); u != "" {
			if frag != "" {
				u = u + "#" + slugFragment(frag)
			}
			return []byte(fmt.Sprintf("[%s](%s)", escapeMD(display), u))
		}
		return []byte(escapeMD(display))
	})
	return src
}

func splitWikilink(s string) (target, display string) {
	if i := strings.Index(s, "|"); i >= 0 {
		return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:])
	}
	t := strings.TrimSpace(s)
	return t, t
}

func splitFragment(s string) (base, frag string) {
	if i := strings.Index(s, "#"); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func slugFragment(s string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(s), " ", "-"))
}

func isImageRef(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".avif"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func filepath_Base(p string) string {
	// Use forward slashes since Obsidian wikilinks always use /.
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// escapeMD escapes characters that would otherwise break out of an inline
// markdown link's display text.
func escapeMD(s string) string {
	return strings.NewReplacer("[", `\[`, "]", `\]`).Replace(s)
}

// articleView wraps an article for template rendering. Embedding lets
// templates keep using {{.Title}}, {{.URL}}, etc. while we shadow Parse with
// a per-request value (avoids racing on Article.Parse across goroutines).
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
