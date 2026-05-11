package server

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	articlepkg "github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/blog"
	"github.com/sbraveyoung/gobog/src/config"
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
// blog's wiki index. Math blocks are stashed before goldmark and restored
// afterwards so backslash row separators (`\\` in `\begin{matrix}…\end{matrix}`)
// survive intact for client-side MathJax.
func renderArticleHTML(article *articlepkg.Article) (string, error) {
	if cached, ok := article.CachedHTML(); ok {
		return cached, nil
	}
	src := append([]byte("## "+article.Title+"\n"), article.Content...)
	src = expandWikilinks(src, blog.Blog.Wiki())

	src, mathStash := protectMath(src)

	var buf bytes.Buffer
	if err := mdRenderer.Convert(src, &buf); err != nil {
		return "", fmt.Errorf("render %q: %w", article.URL, err)
	}
	out := restoreMath(buf.String(), mathStash)

	article.StoreHTML(out)
	return out, nil
}

// Block-level math: $$...$$ across multiple lines, and the LaTeX
// \[...\] form. Inline $...$ is intentionally NOT swapped — too many
// false positives on prose containing $ (shell snippets, currency).
// Authors who want inline math can use \(...\) which is also captured.
var (
	mathBlockRE  = regexp.MustCompile(`(?s)\$\$.*?\$\$`)
	mathParenRE  = regexp.MustCompile(`(?s)\\\(.*?\\\)`)
	mathBracketRE = regexp.MustCompile(`(?s)\\\[.*?\\\]`)
)

// protectMath replaces each math block with an opaque ASCII token so
// goldmark's backslash / underscore handling doesn't mangle the LaTeX
// inside. The mapping is returned so restoreMath can put the originals
// back into the rendered HTML.
//
// Token format: `xxMATHJAXBLOCK<n>ENDxx`. Plain ASCII so goldmark passes
// it through unchanged, distinctive enough to be impossibly-rare in
// natural prose. Two `x` letters at each end keep goldmark from
// treating any single character as syntactically meaningful.
func protectMath(src []byte) ([]byte, map[string]string) {
	stash := make(map[string]string)
	i := 0
	repl := func(m []byte) []byte {
		key := fmt.Sprintf("xxMATHJAXBLOCK%dENDxx", i)
		i++
		stash[key] = string(m)
		return []byte(key)
	}
	out := mathBlockRE.ReplaceAllFunc(src, repl)
	out = mathBracketRE.ReplaceAllFunc(out, repl)
	out = mathParenRE.ReplaceAllFunc(out, repl)
	return out, stash
}

func restoreMath(html string, stash map[string]string) string {
	for k, v := range stash {
		// The token survives literally through goldmark — no escaping
		// needed for replacement.
		html = strings.ReplaceAll(html, k, v)
	}
	return html
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

// SiteMeta is the bundle every page needs in its header / footer: the blog
// title, top-level pages for nav, the configured author, etc. Built fresh
// per request from the live blog state so a content reload picks up new
// pages without restart.
type SiteMeta struct {
	Title       string
	Subtitle    string
	Description string
	Author      string
	Domain      string
	Year        int
	Pages       articlepkg.Articles // top-level pages for the site nav
	Comments    CommentsMeta        // empty struct when comments are disabled
}

// CommentsMeta mirrors config.CommentsConfig but is template-friendly
// (Enabled stays bool; everything else is string so templates can do
// {{ .Site.Comments.Repo }} without type fiddling). Sensible defaults
// fill in for empty mapping / input_position / reactions so users can
// leave those blank in the config.
type CommentsMeta struct {
	Enabled          bool
	Provider         string
	Repo             string
	RepoID           string
	Category         string
	CategoryID       string
	Mapping          string
	ReactionsEnabled string
	InputPosition    string
}

func newCommentsMeta() CommentsMeta {
	c := config.C.Comments
	if !c.Enabled {
		return CommentsMeta{}
	}
	mapping := c.Mapping
	if mapping == "" {
		mapping = "pathname"
	}
	reactions := c.ReactionsEnabled
	if reactions == "" {
		reactions = "1"
	}
	input := c.InputPosition
	if input == "" {
		input = "bottom"
	}
	provider := c.Provider
	if provider == "" {
		provider = "giscus"
	}
	return CommentsMeta{
		Enabled:          true,
		Provider:         provider,
		Repo:             c.Repo,
		RepoID:           c.RepoID,
		Category:         c.Category,
		CategoryID:       c.CategoryID,
		Mapping:          mapping,
		ReactionsEnabled: reactions,
		InputPosition:    input,
	}
}

func newSiteMeta() SiteMeta {
	return SiteMeta{
		Title:       blog.Blog.Name,
		Subtitle:    blog.Blog.SubName,
		Description: blog.Blog.Description,
		Author:      blog.Blog.Author,
		Domain:      blog.Blog.Domain,
		Year:        time.Now().Year(),
		Pages:       blog.Blog.Articles(blog.BlogTypes["page"]),
		Comments:    newCommentsMeta(),
	}
}

// indexView is what the home template receives. SiteMeta lives under .Site so
// templates can disambiguate from per-page metadata (group titles, etc.).
type indexView struct {
	Site   SiteMeta
	Groups articlepkg.Articles
}

func newIndexView(groups articlepkg.Articles) indexView {
	return indexView{Site: newSiteMeta(), Groups: groups}
}

// groupView is what group.html receives for a group landing page (a series,
// a tag, search results, etc.). GroupTitle + GroupURL describe the group
// itself; List is what to enumerate.
type groupView struct {
	Site       SiteMeta
	GroupTitle string
	GroupURL   string
	List       articlepkg.Articles
}

func newGroupView(group *articlepkg.Article, list articlepkg.Articles) groupView {
	v := groupView{Site: newSiteMeta(), List: list}
	if group != nil {
		v.GroupTitle = group.Title
		v.GroupURL = group.URL
	}
	return v
}

// articleView wraps an article for template rendering. Embedding *Article
// lets templates keep using {{.Title}}, {{.URL}}, etc. while we shadow Parse
// with a per-request value (avoids racing on Article.Parse across goroutines).
//
// .Site holds the site-wide bundle so post.html can build the same header /
// footer as index.html without re-fetching globals.
type articleView struct {
	*articlepkg.Article
	Site        SiteMeta
	Parse       string
	Domain      string
	Canonical   string
	SiteTitle   string
	Description string
	Pinned      bool
	Private     bool
	AI          bool
	AILabel     string
	CoverURL    string
}

func newArticleView(a *articlepkg.Article, parse, domain string) articleView {
	desc := a.Summary
	if desc == "" {
		desc = a.Description
	}
	site := newSiteMeta()
	if domain == "" {
		domain = site.Domain
	}
	return articleView{
		Article:     a,
		Site:        site,
		Parse:       parse,
		Domain:      domain,
		Canonical:   domain + a.URL,
		SiteTitle:   a.Title,
		Description: desc,
		Pinned:      a.IsPinned(),
		Private:     a.IsPrivate(),
		AI:          a.IsAI(),
		AILabel:     a.AILabel(),
		CoverURL:    coverURL(a.Cover),
	}
}

// coverURL turns a front-matter `cover:` value into a usable URL. Absolute
// URLs (http(s)://, /...) pass through unchanged; bare names like "cover.png"
// get prefixed with /image/ so the existing image handler can look them up
// in the wiki index.
func coverURL(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") || strings.HasPrefix(v, "/") {
		return v
	}
	return "/image/" + v
}
