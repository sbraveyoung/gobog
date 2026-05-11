package blog

import (
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/astaxie/beego/logs"
	"github.com/fsnotify/fsnotify"
	articlepkg "github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/config"
)

// BlogTypes maps a logical type to its directory under <source>.
//   - "post"  → <source>/post/   (recursively scanned, becomes /post/...)
//   - "page"  → <source>/pages/  (each <name>.md becomes /<name>, e.g.
//                                  pages/about.md → /about)
//
// Pages replace the old hardcoded "about" page. Any number of top-level
// pages can live under pages/; they all show in the site header nav.
var BlogTypes = map[string]string{
	"post": "post",
	"page": "pages",
}

var Blog *BlogST

type BlogST struct {
	Domain      string
	Name        string
	SubName     string
	Description string
	Author      string
	Theme       string

	mu       sync.RWMutex
	articles map[string]articlepkg.Articles
	byTag    map[string]articlepkg.Articles
	wiki     *WikiIndex
}

// WikiIndex powers Obsidian wikilink + image-embed resolution. Built once per
// Reload(); read-only afterwards.
type WikiIndex struct {
	// notes maps a lookup key (lowercased note title or relative path without
	// .md extension) to the article's URL. Multiple keys can resolve to the
	// same article: the file's basename, the relative path, the front-matter
	// title, and any of those with separators normalized.
	notes map[string]string
	// images maps a basename (lowercased, with or without extension) to the
	// disk path of an asset relative to <source>.
	images map[string]string
}

func newWikiIndex() *WikiIndex {
	return &WikiIndex{notes: make(map[string]string), images: make(map[string]string)}
}

// NewWikiIndexForTesting builds a WikiIndex from the given maps. The note keys
// are normalized internally so callers can pass titles or paths verbatim.
func NewWikiIndexForTesting(notes, images map[string]string) *WikiIndex {
	w := newWikiIndex()
	for k, v := range notes {
		w.notes[normalizeKey(k)] = v
	}
	for k, v := range images {
		w.images[strings.ToLower(k)] = v
	}
	return w
}

// ResolveNote returns the URL for [[link]] target or empty string if unknown.
func (w *WikiIndex) ResolveNote(target string) string {
	if w == nil {
		return ""
	}
	t := normalizeKey(target)
	if u, ok := w.notes[t]; ok {
		return u
	}
	// Try without leading folders ("foo/bar" → "bar").
	if i := strings.LastIndex(t, "/"); i >= 0 {
		if u, ok := w.notes[t[i+1:]]; ok {
			return u
		}
	}
	return ""
}

// ResolveImage returns the relative-to-source disk path for an image basename.
func (w *WikiIndex) ResolveImage(basename string) string {
	if w == nil {
		return ""
	}
	return w.images[strings.ToLower(basename)]
}

func normalizeKey(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".md")
	s = strings.ToLower(s)
	return s
}

func init() {
	Blog = &BlogST{
		Domain:      config.C.Blog.Domain,
		Name:        config.C.Blog.Title,
		SubName:     config.C.Blog.Subtitle,
		Description: config.C.Blog.Description,
		Author:      config.C.Blog.Author,
		Theme:       config.C.Blog.Theme,
		articles:    make(map[string]articlepkg.Articles),
		byTag:       make(map[string]articlepkg.Articles),
		wiki:        newWikiIndex(),
	}
	if config.Testing {
		return
	}

	if err := Blog.Reload(); err != nil {
		logs.Error("initial blog scan:", err)
		os.Exit(1)
	}

	if config.ExportDir == "" {
		Blog.startWatcher()
	}
}

// SetForTesting replaces the in-memory article index.
func (b *BlogST) SetForTesting(byType map[string]articlepkg.Articles) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.articles = byType
	b.byTag = buildTagIndex(byType[BlogTypes["post"]])
	b.wiki = newWikiIndex()
}

// Wiki returns the current wiki index. Safe for read; treat as immutable.
func (b *BlogST) Wiki() *WikiIndex {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.wiki
}

// Reload re-scans <source> and atomically replaces the in-memory state.
//
// Layout: <source>/ contains arbitrarily nested .md files. Each subdirectory
// becomes a group; arbitrary depth is supported. Any .md placed under
// <source>/about/ is treated as the single about page (first by sort order).
// Anywhere else under <source> is a post.
//
// Missing front-matter (id / url / title / create_time) is auto-filled and
// persisted back to the source file the first time the scanner sees a note,
// matching the original gobog product design. URL generation strategy is
// controlled by [blog].layout: "vault" (default) uses slugified path,
// "legacy" uses crc32 hex like the original gobog (kept so already-published
// hex URLs stay stable for sites that switched over).
func (b *BlogST) Reload() error {
	source := config.C.Blog.Source

	articles := make(map[string]articlepkg.Articles)
	wiki := newWikiIndex()

	postRoot := filepath.Join(source, BlogTypes["post"])
	posts, err := loadDir(postRoot, postRoot, "/post", true)
	if err != nil {
		return err
	}
	articles[BlogTypes["post"]] = posts

	pages, err := loadPages(filepath.Join(source, BlogTypes["page"]))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	articles[BlogTypes["page"]] = pages

	indexNotes(wiki, articles[BlogTypes["post"]])
	indexNotes(wiki, pages)
	if err := indexImages(wiki, source); err != nil {
		logs.Warn("image index:", err)
	}

	tagIndex := buildTagIndex(articles[BlogTypes["post"]])

	b.mu.Lock()
	b.articles = articles
	b.byTag = tagIndex
	b.wiki = wiki
	b.mu.Unlock()
	return nil
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// useLegacyURLs reports whether [blog].layout asks for legacy-style URLs
// (`/post/<crc32-hex>/<crc32-hex>`) for newly-encountered articles.
// "auto" / "vault" / empty → false (slug URLs); "legacy" → true.
func useLegacyURLs() bool {
	return strings.EqualFold(strings.TrimSpace(config.C.Blog.Layout), "legacy")
}

// shouldExcludeTop reports whether a top-level directory name in vault mode
// should be skipped — both via the user's [blog].exclude_dirs list and via
// the implicit "about" exclusion (handled separately) and the dotfile
// rule. Comparison is case-insensitive on basename.
func shouldExcludeTop(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasPrefix(name, ".") {
		return true
	}
	for _, ex := range config.C.Blog.ExcludeDirs {
		if strings.EqualFold(strings.TrimSpace(ex), lower) {
			return true
		}
	}
	return false
}

// loadDir loads a directory into an Articles slice.
//
//	dirPath   — the directory being walked.
//	walkRoot  — the root the walk started from (e.g. <source>/post/). Used
//	            to compute path-relative URLs without doubling the
//	            urlPrefix segment.
//	urlPrefix — what the resulting article URLs start with (e.g. "/post").
//	isRoot    — true only for the call on walkRoot itself; controls
//	            top-level filters like [blog].exclude_dirs.
func loadDir(dirPath, walkRoot, urlPrefix string, isRoot bool) (articlepkg.Articles, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	var list articlepkg.Articles
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		full := filepath.Join(dirPath, name)
		if e.IsDir() {
			if isRoot && shouldExcludeTop(name) {
				continue
			}
			child, err := loadGroupDir(full, walkRoot, urlPrefix)
			if err != nil {
				logs.Warn("loadGroupDir:", err)
				continue
			}
			if child != nil {
				list = append(list, child)
			}
		} else {
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			art, err := buildArticle(full, walkRoot, urlPrefix, name)
			if err != nil {
				logs.Warn("buildArticle:", err)
				continue
			}
			if art == nil {
				continue
			}
			list = append(list, art)
		}
	}
	sort.Sort(list)
	return list, nil
}

// loadGroupDir walks a non-root directory and produces a group Article whose
// SubArticle list contains the directory's notes (recursively).
func loadGroupDir(dirPath, walkRoot, urlPrefix string) (*articlepkg.Article, error) {
	rel, err := filepath.Rel(walkRoot, dirPath)
	if err != nil {
		return nil, err
	}
	groupURL := urlPrefix + "/" + slugifyPath(rel)

	subs, err := loadDir(dirPath, walkRoot, urlPrefix, false)
	if err != nil {
		return nil, err
	}
	// Drop empty groups — a directory with no publishable .md files (or
	// only sub-groups that all turned out empty) shouldn't appear in the
	// listings or render an empty index page.
	if len(subs) == 0 {
		return nil, nil
	}

	// Pick the group's CreateTime BEFORE we potentially reorder. The
	// default loadDir sort is date-desc, so subs[0] holds the newest
	// item's date — that's what we want to surface in the home listing
	// so series with a fresh chapter float back up.
	groupDate := subs[0].CreateTime

	// Series detection: if every leaf has a numeric order signal
	// (front-matter `order:` or digit-prefix filename), re-sort the
	// sub-list ascending so chapter 1 reads before chapter 2.
	if isSequentialSeries(subs) {
		sort.SliceStable(subs, func(i, j int) bool {
			return subs[i].SeriesOrderKey() < subs[j].SeriesOrderKey()
		})
	}

	g := &articlepkg.Article{}
	g.Source = dirPath
	g.Title = filepath.Base(dirPath)
	g.Id = articlepkg.SlugOrHash(g.Title)
	g.URL = groupURL
	g.CreateTime = groupDate
	g.SubArticle = subs
	return g, nil
}

// isSequentialSeries returns true when every leaf in `subs` carries a
// SeriesOrderKey ≥ 0. Empty groups, or groups whose direct children are
// themselves groups, fall through to false — sub-groups participate in
// the parent's date sort. Mixed groups (some leaves with order, some
// without) also fall back to date order so a single mis-named file
// doesn't silently scramble the rest of the listing.
func isSequentialSeries(subs articlepkg.Articles) bool {
	leaves := 0
	withOrder := 0
	for _, a := range subs {
		if len(a.SubArticle) > 0 {
			continue
		}
		leaves++
		if a.SeriesOrderKey() >= 0 {
			withOrder++
		}
	}
	return leaves >= 2 && withOrder == leaves
}

// buildArticle parses the .md at path and fills in any missing meta
// (id / url / title / create_time) deterministically from the path / file
// stat / [blog].layout, then persists the filled-in front-matter back to
// disk. Persisting on first scan is part of the original gobog product
// design — once a note is on the site, its id and url stop drifting even
// if the file is later renamed or moved.
//
// Returns (nil, nil) when the article should be skipped (draft / hidden
// without the include_drafts / include_hidden flag).
func buildArticle(path, walkRoot, urlPrefix, fileName string) (*articlepkg.Article, error) {
	a, err := articlepkg.ParseFile(path)
	if err != nil {
		return nil, err
	}
	if a.IsDraft() && !config.C.Blog.IncludeDrafts {
		return nil, nil
	}
	if a.IsHidden() && !config.C.Blog.IncludeHidden {
		return nil, nil
	}

	rel, err := filepath.Rel(walkRoot, path)
	if err != nil {
		return nil, err
	}
	rel = strings.TrimSuffix(rel, ".md")

	updated := false
	if a.Title == "" {
		a.Title = strings.TrimSuffix(fileName, ".md")
		updated = true
	}
	if a.CreateTime == "" {
		if fi, err := os.Stat(path); err == nil {
			a.CreateTime = fi.ModTime().Format(articlepkg.TIME_LAYOUT)
		} else {
			a.CreateTime = time.Now().Format(articlepkg.TIME_LAYOUT)
		}
		updated = true
	}
	if a.Id == "" {
		a.Id = generateID(rel, fileName, a.Content)
		updated = true
	}
	if a.URL == "" {
		a.URL = generateURL(rel, urlPrefix, a.Id, a.Content)
		updated = true
	}

	if updated {
		if err := articlepkg.RewriteFrontMatter(a); err != nil {
			// Persisting is best-effort: a read-only filesystem shouldn't
			// take the whole site down. Log and serve the in-memory copy.
			logs.Warn("persist front-matter:", err)
		}
	}
	return a, nil
}

// generateID picks a stable identifier for the article when front-matter
// doesn't carry one. Defaults to a slug of the file basename; under
// [blog].layout="legacy" returns the crc32 hex of the body content (which
// is what the original gobog used).
func generateID(rel, fileName string, content []byte) string {
	if useLegacyURLs() {
		return articlepkg.CalcID(content)
	}
	return articlepkg.SlugOrHash(strings.TrimSuffix(fileName, ".md"))
}

// generateURL builds the article URL. Default: slugified relative path.
// Legacy: `<urlPrefix>/<crc32(parent-dir)>/<id>` for nested files (matches
// the original gobog 1-level group convention) and `<urlPrefix>/<id>` for
// notes at <source>/.
func generateURL(rel, urlPrefix, id string, content []byte) string {
	if useLegacyURLs() {
		parent := filepath.Dir(rel)
		if parent == "." || parent == "" || parent == string(filepath.Separator) {
			return urlPrefix + "/" + id
		}
		return urlPrefix + "/" + articlepkg.CalcID([]byte(filepath.Base(parent))) + "/" + id
	}
	return urlPrefix + "/" + slugifyPath(rel)
}

func slugifyPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, articlepkg.SlugOrHash(p))
	}
	return strings.Join(out, "/")
}

// loadPages walks <source>/pages/ and produces one Article per .md file.
// Each file's URL is /<slug-of-basename> so pages/about.md → /about,
// pages/contact.md → /contact, etc. Front-matter `url:` overrides the
// auto-generated URL when present.
//
// Unlike posts, pages don't nest — only top-level *.md files are picked up.
// Sub-directories under pages/ are ignored (kept for the user to stash
// drafts or attachments without exposing them).
func loadPages(dir string) (articlepkg.Articles, error) {
	if !dirExists(dir) {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var list articlepkg.Articles
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		full := filepath.Join(dir, e.Name())
		a, err := articlepkg.ParseFile(full)
		if err != nil {
			logs.Warn("page parse:", err)
			continue
		}
		basename := strings.TrimSuffix(e.Name(), ".md")
		updated := false
		if a.Title == "" {
			a.Title = basename
			updated = true
		}
		slug := articlepkg.SlugOrHash(basename)
		if a.URL == "" {
			a.URL = "/" + slug
			updated = true
		}
		if a.Id == "" {
			a.Id = slug
			updated = true
		}
		if a.CreateTime == "" {
			if fi, err := os.Stat(full); err == nil {
				a.CreateTime = fi.ModTime().Format(articlepkg.TIME_LAYOUT)
				updated = true
			}
		}
		if updated {
			if err := articlepkg.RewriteFrontMatter(a); err != nil {
				logs.Warn("persist page front-matter:", err)
			}
		}
		list = append(list, a)
	}
	sort.Sort(list)
	return list, nil
}

// indexNotes registers each article in the wiki index under multiple keys so
// `[[note title]]`, `[[basename]]`, `[[Folder/Note]]` all resolve.
func indexNotes(w *WikiIndex, list articlepkg.Articles) {
	for _, a := range list {
		register := func(key string) {
			if key == "" {
				return
			}
			w.notes[normalizeKey(key)] = a.URL
		}
		if a.Source != "" {
			base := filepath.Base(a.Source)
			register(strings.TrimSuffix(base, ".md"))
			source := config.C.Blog.Source
			if rel, err := filepath.Rel(source, a.Source); err == nil {
				rel = strings.TrimSuffix(filepath.ToSlash(rel), ".md")
				register(rel)
			}
		}
		register(a.Title)
		if len(a.SubArticle) > 0 {
			indexNotes(w, a.SubArticle)
		}
	}
}

// indexImages walks <source> for non-.md files and registers them by basename.
// /image/<basename> requests will resolve through this map.
func indexImages(w *WikiIndex, source string) error {
	return filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") && path != source {
				return filepath.SkipDir
			}
			return nil
		}
		name := info.Name()
		if strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		key := strings.ToLower(name)
		// First write wins; longer paths don't override earlier shallow ones
		// (matches Obsidian's behavior of preferring closer files, but we
		// don't have a "current note" here).
		if _, ok := w.images[key]; !ok {
			w.images[key] = rel
		}
		return nil
	})
}

func buildTagIndex(posts articlepkg.Articles) map[string]articlepkg.Articles {
	idx := make(map[string]articlepkg.Articles)
	var walk func(articlepkg.Articles)
	walk = func(list articlepkg.Articles) {
		for _, a := range list {
			for _, t := range a.Tags {
				idx[t] = append(idx[t], a)
			}
			if len(a.SubArticle) > 0 {
				walk(a.SubArticle)
			}
		}
	}
	walk(posts)
	for k := range idx {
		sort.Sort(idx[k])
	}
	return idx
}

func (b *BlogST) Articles(typ string) articlepkg.Articles {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.articles[typ]
}

func (b *BlogST) AllPosts() articlepkg.Articles {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var out articlepkg.Articles
	var walk func(articlepkg.Articles)
	walk = func(list articlepkg.Articles) {
		for _, a := range list {
			if len(a.SubArticle) > 0 {
				walk(a.SubArticle)
				continue
			}
			out = append(out, a)
		}
	}
	walk(b.articles[BlogTypes["post"]])
	return out
}

func (b *BlogST) Groups() articlepkg.Articles {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.articles[BlogTypes["post"]]
}

func (b *BlogST) FindByURL(url string) *articlepkg.Article {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var search func(articlepkg.Articles) *articlepkg.Article
	search = func(list articlepkg.Articles) *articlepkg.Article {
		for _, a := range list {
			if a.URL == url {
				return a
			}
			if hit := search(a.SubArticle); hit != nil {
				return hit
			}
		}
		return nil
	}
	for _, list := range b.articles {
		if hit := search(list); hit != nil {
			return hit
		}
	}
	return nil
}

func (b *BlogST) Tags() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	tags := make([]string, 0, len(b.byTag))
	for t := range b.byTag {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

func (b *BlogST) PostsByTag(tag string) articlepkg.Articles {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.byTag[tag]
}

func (b *BlogST) LatestUpdate() time.Time {
	var latest time.Time
	for _, a := range b.AllPosts() {
		t, err := time.Parse(articlepkg.TIME_LAYOUT, a.CreateTime)
		if err != nil {
			continue
		}
		if t.After(latest) {
			latest = t
		}
	}
	return latest
}

func (b *BlogST) startWatcher() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		logs.Warn("fsnotify:", err)
		return
	}

	addRecursive := func(root string) {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || !info.IsDir() {
				return nil
			}
			if strings.HasPrefix(info.Name(), ".") && p != root {
				return filepath.SkipDir
			}
			_ = w.Add(p)
			return nil
		})
	}
	addRecursive(config.C.Blog.Source)

	go func() {
		debounce := time.NewTimer(time.Hour)
		debounce.Stop()
		pending := false
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if !shouldReload(ev) {
					continue
				}
				if ev.Op&fsnotify.Create != 0 {
					if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
						_ = w.Add(ev.Name)
					}
				}
				pending = true
				debounce.Reset(300 * time.Millisecond)
			case <-debounce.C:
				if !pending {
					continue
				}
				pending = false
				if err := b.Reload(); err != nil {
					logs.Warn("reload:", err)
				} else {
					logs.Info("blog reloaded")
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				logs.Warn("fsnotify error:", err)
			}
		}
	}()
}

func shouldReload(ev fsnotify.Event) bool {
	if ev.Op == 0 {
		return false
	}
	base := pathpkg.Base(ev.Name)
	if strings.HasPrefix(base, ".") {
		return false
	}
	if strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") {
		return false
	}
	if strings.HasSuffix(base, ".md") {
		return true
	}
	if ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		return true
	}
	return false
}
