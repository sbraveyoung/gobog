package blog

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	articlepkg "github.com/sbraveyoung/gobog/src/article"
)

// collectUnpublishedRoutes distinguishes withheld content from misspelled links.
// It never adds notes to public indexes or rewrites their source metadata.
func collectUnpublishedRoutes(source string, articles map[string]articlepkg.Articles) (map[string]bool, error) {
	available := map[string]bool{}
	var register func(articlepkg.Articles)
	register = func(list articlepkg.Articles) {
		for _, a := range list {
			available[strings.TrimSuffix(a.URL, "/")] = true
			register(a.SubArticle)
		}
	}
	for _, list := range articles {
		register(list)
	}
	unavailable := map[string]bool{}
	mark := func(route string) {
		route = strings.TrimSuffix(route, "/")
		if !available[route] {
			unavailable[route] = true
		}
	}
	postRoot := filepath.Join(source, BlogTypes["post"])
	err := filepath.WalkDir(postRoot, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if p == postRoot {
			return nil
		}
		rel, err := filepath.Rel(postRoot, p)
		if err != nil {
			return err
		}
		if strings.HasPrefix(e.Name(), ".") || (e.IsDir() && !strings.Contains(rel, string(filepath.Separator)) && shouldExcludeTop(e.Name())) {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if e.IsDir() {
			mark("/post/" + slugifyPath(rel))
			return nil
		}
		if filepath.Ext(p) != ".md" || e.Name() == FolderPolicyFile {
			return nil
		}
		a, err := articlepkg.ParseFile(p)
		// Invalid notes in an excluded subtree stay excluded. Active notes have
		// already been validated by loadDir, and broken unknown links still fail CI.
		if err != nil {
			return nil
		}
		if a.URL != "" {
			mark(a.URL)
		} else {
			rel = strings.TrimSuffix(rel, ".md")
			id := a.Id
			if id == "" {
				id = generateID(rel, e.Name(), a.Content)
			}
			mark(generateURL(rel, "/post", id, a.Content))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	pageRoot := filepath.Join(source, BlogTypes["page"])
	entries, err := os.ReadDir(pageRoot)
	if os.IsNotExist(err) {
		return unavailable, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || e.Type()&os.ModeSymlink != 0 || strings.HasPrefix(e.Name(), ".") || e.Name() == FolderPolicyFile || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		a, err := articlepkg.ParseFile(filepath.Join(pageRoot, e.Name()))
		if err != nil {
			continue
		}
		if a.URL != "" {
			mark(a.URL)
		} else {
			mark("/" + articlepkg.SlugOrHash(strings.TrimSuffix(e.Name(), ".md")))
		}
	}
	return unavailable, nil
}

func (b *BlogST) IsUnpublishedURL(route string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.unpublished[strings.TrimSuffix(route, "/")]
}
