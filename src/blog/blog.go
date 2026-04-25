package blog

import (
	"os"
	pathpkg "path"
	"sort"
	"strings"
	"sync"
	"time"

	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/SmartBrave/gobog/src/config"
	"github.com/astaxie/beego/logs"
	"github.com/fsnotify/fsnotify"
)

var (
	BlogTypes = map[string]string{
		"post":  "post",
		"about": "about",
	}
)

var (
	Blog *BlogST
)

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
	}
	if config.Testing {
		// Tests should populate state via SetForTesting if they need it.
		return
	}

	if err := Blog.Reload(); err != nil {
		logs.Error("initial blog scan:", err)
		os.Exit(1)
	}

	// Hot reload on .md changes when running as a server (we still scan
	// once for export mode, but a watcher there would just leak goroutines).
	if config.ExportDir == "" {
		Blog.startWatcher()
	}
}

// SetForTesting replaces the in-memory article index. Test-only helper.
func (b *BlogST) SetForTesting(byType map[string]articlepkg.Articles) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.articles = byType
	b.byTag = buildTagIndex(byType[BlogTypes["post"]])
}

// Reload re-scans the source directory and atomically replaces the in-memory
// article index. Safe to call concurrently with reads.
func (b *BlogST) Reload() error {
	articles := make(map[string]articlepkg.Articles)
	for _, tYpe := range BlogTypes {
		rootPath := pathpkg.Join(config.C.Blog.Source, tYpe)
		root, err := os.Open(rootPath)
		if err != nil {
			return err
		}
		names, err := root.Readdirnames(-1)
		root.Close()
		if err != nil {
			return err
		}

		var list articlepkg.Articles
		for _, name := range names {
			if strings.HasPrefix(name, ".") {
				continue
			}
			path := pathpkg.Join(rootPath, name)
			fileInfo, err := os.Lstat(path)
			if err != nil {
				logs.Warn("os.Lstat err:", err)
				continue
			}
			if fileInfo.IsDir() {
				art, err := articlepkg.NewArticle(path, articlepkg.DIR, "/"+tYpe)
				if err != nil {
					logs.Warn("NewArticle dir err:", err, " path:", path)
					continue
				}
				if err := loadGroup(art, path); err != nil {
					logs.Warn("loadGroup err:", err)
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
				art, err := articlepkg.NewArticle(path, articlepkg.ARTICLE, "/"+tYpe)
				if err != nil {
					logs.Warn("NewArticle err:", err)
					continue
				}
				if art.IsDraft() && !config.C.Blog.IncludeDrafts {
					continue
				}
				list = append(list, art)
			}
		}
		sort.Sort(list)
		articles[tYpe] = list
	}

	tagIndex := buildTagIndex(articles[BlogTypes["post"]])

	b.mu.Lock()
	b.articles = articles
	b.byTag = tagIndex
	b.mu.Unlock()
	return nil
}

func loadGroup(group *articlepkg.Article, dir string) error {
	subRoot, err := os.Open(dir)
	if err != nil {
		return err
	}
	subNames, err := subRoot.Readdirnames(-1)
	subRoot.Close()
	if err != nil {
		return err
	}

	for _, subName := range subNames {
		if strings.HasPrefix(subName, ".") {
			continue
		}
		subPath := pathpkg.Join(dir, subName)
		if !strings.HasSuffix(subPath, ".md") {
			continue
		}
		sub, err := articlepkg.NewArticle(subPath, articlepkg.ARTICLE, group.URL)
		if err != nil {
			logs.Warn("sub article err:", err, " path:", subPath)
			continue
		}
		if sub.IsDraft() && !config.C.Blog.IncludeDrafts {
			continue
		}
		group.SubArticle = append(group.SubArticle, sub)
	}
	sort.Sort(group.SubArticle)
	return nil
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

// Articles returns the live slice for the given type. Callers must not mutate
// the returned slice; treat it as read-only.
func (b *BlogST) Articles(typ string) articlepkg.Articles {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.articles[typ]
}

// AllPosts returns every post (groups flattened to leaves only). Useful for
// feeds, sitemap and static export.
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

// Groups returns top-level entries (groups + standalone posts) for the post
// type, in their stored order. Useful for the index page and static export.
func (b *BlogST) Groups() articlepkg.Articles {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.articles[BlogTypes["post"]]
}

// FindByURL walks both the post and about types and returns the article whose
// URL exactly matches.
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

// Tags returns all known tags in stable (alphabetical) order.
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

// PostsByTag returns the post list for the given tag.
func (b *BlogST) PostsByTag(tag string) articlepkg.Articles {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.byTag[tag]
}

// LatestUpdate returns the most recent post create_time, or zero value if no
// posts exist. Used for sitemap lastmod and feed updated.
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
		logs.Warn("fsnotify NewWatcher err:", err)
		return
	}

	addRecursive := func(root string) {
		_ = w.Add(root)
		for _, t := range BlogTypes {
			sub := pathpkg.Join(root, t)
			_ = w.Add(sub)
			entries, err := os.ReadDir(sub)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					_ = w.Add(pathpkg.Join(sub, e.Name()))
				}
			}
		}
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
					logs.Warn("reload err:", err)
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
	// Directory ops still warrant a reload (new group, deleted group).
	if ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
		return true
	}
	return false
}
