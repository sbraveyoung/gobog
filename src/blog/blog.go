package blog

import (
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/SmartBrave/gobog/src/config"
	"github.com/astaxie/beego/logs"
	"github.com/fsnotify/fsnotify"
)

// BlogTypes maps a logical type to the URL prefix used in routes / templates.
// "post" gets all top-level + nested notes; "about" is a single note (the
// first .md file under <source>/about/, if present).
var BlogTypes = map[string]string{
	"post":  "post",
	"about": "about",
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
// Two layouts are supported:
//
//   - Legacy gobog: <source>/post/... (group dirs allowed, 1 level deep) and
//     <source>/about/*.md. Activated when <source>/post/ exists OR when
//     [blog].layout = "legacy".
//   - Obsidian-style folder: <source>/ contains arbitrarily nested .md files.
//     Each subdirectory becomes a group; arbitrary depth is supported. Any
//     .md placed under <source>/about/ is treated as the single about page
//     (first by sort order). Anywhere else under <source> is a post.
//     Use [blog].layout = "vault" to force this even when a "post" folder
//     happens to exist in the vault.
func (b *BlogST) Reload() error {
	source := config.C.Blog.Source

	useLegacy := pickLegacyMode(source)

	articles := make(map[string]articlepkg.Articles)
	wiki := newWikiIndex()

	if useLegacy {
		posts, err := loadLegacyPosts(filepath.Join(source, "post"))
		if err != nil {
			return err
		}
		articles[BlogTypes["post"]] = posts
	} else {
		posts, err := loadVaultPosts(source)
		if err != nil {
			return err
		}
		articles[BlogTypes["post"]] = posts
	}

	abouts, err := loadAbouts(filepath.Join(source, BlogTypes["about"]))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	articles[BlogTypes["about"]] = abouts

	indexNotes(wiki, articles[BlogTypes["post"]])
	indexNotes(wiki, abouts)
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

// pickLegacyMode honors [blog].layout when set ("legacy" / "vault"); the
// default ("auto" / empty) falls back to detecting <source>/post/ on disk.
func pickLegacyMode(source string) bool {
	switch strings.ToLower(strings.TrimSpace(config.C.Blog.Layout)) {
	case "legacy":
		return true
	case "vault":
		return false
	}
	return dirExists(filepath.Join(source, "post"))
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

// loadVaultPosts walks source recursively, treating each .md file as a post
// and each subdirectory as a group. Files under "about/" are excluded (the
// caller picks them up separately).
func loadVaultPosts(source string) (articlepkg.Articles, error) {
	return loadDir(source, source, "/post", true)
}

// loadDir loads a directory into an Articles slice. dirPath is the directory
// being walked; sourceRoot is the configured <source> (used to compute
// relative URLs). urlPrefix is what the resulting article URLs start with.
// excludeAbout=true skips the top-level "about" subdirectory.
func loadDir(dirPath, sourceRoot, urlPrefix string, excludeAbout bool) (articlepkg.Articles, error) {
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
			if dirPath == sourceRoot {
				if excludeAbout && name == "about" {
					continue
				}
				if shouldExcludeTop(name) {
					continue
				}
			}
			child, err := loadGroupDir(full, sourceRoot, urlPrefix)
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
			art, err := buildArticle(full, sourceRoot, urlPrefix, name)
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
func loadGroupDir(dirPath, sourceRoot, urlPrefix string) (*articlepkg.Article, error) {
	rel, err := filepath.Rel(sourceRoot, dirPath)
	if err != nil {
		return nil, err
	}
	groupURL := urlPrefix + "/" + slugifyPath(rel)

	subs, err := loadDir(dirPath, sourceRoot, urlPrefix, false)
	if err != nil {
		return nil, err
	}
	if len(subs) == 0 {
		return nil, nil
	}

	g := &articlepkg.Article{}
	g.Source = dirPath
	g.Title = filepath.Base(dirPath)
	g.Id = articlepkg.SlugOrHash(g.Title)
	g.URL = groupURL
	g.CreateTime = subs[0].CreateTime // newest sub becomes the group's date
	g.SubArticle = subs
	return g, nil
}

func buildArticle(path, sourceRoot, urlPrefix, fileName string) (*articlepkg.Article, error) {
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
	rel, err := filepath.Rel(sourceRoot, path)
	if err != nil {
		return nil, err
	}
	rel = strings.TrimSuffix(rel, ".md")
	if a.URL == "" {
		a.URL = urlPrefix + "/" + slugifyPath(rel)
	}
	if a.Id == "" {
		a.Id = articlepkg.SlugOrHash(strings.TrimSuffix(fileName, ".md"))
	}
	if a.Title == "" {
		a.Title = strings.TrimSuffix(fileName, ".md")
	}
	if a.CreateTime == "" {
		// Fall back to file mtime so listings sort sensibly even when the
		// author hasn't filled in front-matter.
		if fi, err := os.Stat(path); err == nil {
			a.CreateTime = fi.ModTime().Format(articlepkg.TIME_LAYOUT)
		} else {
			a.CreateTime = time.Now().Format(articlepkg.TIME_LAYOUT)
		}
	}
	return a, nil
}

func slugifyPath(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, articlepkg.SlugOrHash(p))
	}
	return strings.Join(out, "/")
}

// loadLegacyPosts mimics the original gobog scanner: <source>/post/ with
// optional 1-level group subdirectories. Kept so existing deployments keep
// working when <source>/post/ exists.
func loadLegacyPosts(rootPath string) (articlepkg.Articles, error) {
	root, err := os.Open(rootPath)
	if err != nil {
		return nil, err
	}
	names, err := root.Readdirnames(-1)
	root.Close()
	if err != nil {
		return nil, err
	}

	var list articlepkg.Articles
	for _, name := range names {
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := pathpkg.Join(rootPath, name)
		fi, err := os.Lstat(path)
		if err != nil {
			logs.Warn("lstat:", err)
			continue
		}
		if fi.IsDir() {
			art, err := articlepkg.NewArticle(path, articlepkg.DIR, "/post")
			if err != nil {
				logs.Warn("NewArticle dir:", err)
				continue
			}
			if err := loadLegacyGroup(art, path); err != nil {
				logs.Warn("loadLegacyGroup:", err)
				continue
			}
			if len(art.SubArticle) == 0 {
				continue
			}
			list = append(list, art)
		} else {
			if !strings.HasSuffix(path, ".md") {
				continue
			}
			art, err := articlepkg.NewArticle(path, articlepkg.ARTICLE, "/post")
			if err != nil {
				logs.Warn("NewArticle:", err)
				continue
			}
			if art.IsDraft() && !config.C.Blog.IncludeDrafts {
				continue
			}
			if art.IsHidden() && !config.C.Blog.IncludeHidden {
				continue
			}
			list = append(list, art)
		}
	}
	sort.Sort(list)
	return list, nil
}

func loadLegacyGroup(group *articlepkg.Article, dir string) error {
	subRoot, err := os.Open(dir)
	if err != nil {
		return err
	}
	names, err := subRoot.Readdirnames(-1)
	subRoot.Close()
	if err != nil {
		return err
	}
	for _, n := range names {
		if strings.HasPrefix(n, ".") {
			continue
		}
		p := pathpkg.Join(dir, n)
		if !strings.HasSuffix(p, ".md") {
			continue
		}
		sub, err := articlepkg.NewArticle(p, articlepkg.ARTICLE, group.URL)
		if err != nil {
			logs.Warn("sub article:", err)
			continue
		}
		if sub.IsDraft() && !config.C.Blog.IncludeDrafts {
			continue
		}
		if sub.IsHidden() && !config.C.Blog.IncludeHidden {
			continue
		}
		group.SubArticle = append(group.SubArticle, sub)
	}
	sort.Sort(group.SubArticle)
	return nil
}

func loadAbouts(dir string) (articlepkg.Articles, error) {
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
			logs.Warn("about parse:", err)
			continue
		}
		if a.URL == "" {
			a.URL = "/about"
		}
		if a.Title == "" {
			a.Title = strings.TrimSuffix(e.Name(), ".md")
		}
		if a.Id == "" {
			a.Id = articlepkg.SlugOrHash(a.Title)
		}
		if a.CreateTime == "" {
			if fi, err := os.Stat(full); err == nil {
				a.CreateTime = fi.ModTime().Format(articlepkg.TIME_LAYOUT)
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
