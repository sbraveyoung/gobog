package blog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	articlepkg "github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/config"
)

// FolderPolicyFile is editable as an ordinary Obsidian note, but is never an article.
const FolderPolicyFile = "_folder.md"

// FolderHidden applies every ancestor veto, including the blog root. A child
// hidden:false cannot expose content hidden by its parent. Parsing errors abort
// the build even in preview, rather than interpreting an invalid policy as public.
func FolderHidden(dir string) (bool, error) {
	root, err := canonicalFolder(config.C.Blog.Source)
	if err != nil {
		return false, err
	}
	dir, err = canonicalFolder(dir)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, fmt.Errorf("folder %s is outside blog source", dir)
	}
	hidden := false
	for {
		policy := filepath.Join(dir, FolderPolicyFile)
		if info, err := os.Lstat(policy); err == nil {
			if !info.Mode().IsRegular() {
				return false, fmt.Errorf("folder policy %s must be a regular file", policy)
			}
			a, err := articlepkg.ParseFile(policy)
			if err != nil {
				return false, err
			}
			switch strings.ToLower(strings.TrimSpace(a.Hidden)) {
			case "true", "1", "yes", "on":
				hidden = true
			case "false", "0", "no", "off":
			default:
				return false, fmt.Errorf("hidden in %s must be a boolean", policy)
			}
		} else if !os.IsNotExist(err) {
			return false, err
		}
		if dir == root {
			return hidden, nil
		}
		dir = filepath.Dir(dir)
	}
}

func articleVisible(a *articlepkg.Article) bool {
	return (!a.IsDraft() || config.C.Blog.IncludeDrafts) &&
		(!a.IsHidden() || config.C.Blog.IncludeHidden) &&
		(!a.IsPrivate() || config.ExportDir == "")
}

// Canonicalize even a not-yet-created pages directory. macOS /var and /tmp
// aliases must compare equal to /private/var and /private/tmp.
func canonicalFolder(dir string) (string, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return real, nil
	}
	if !os.IsNotExist(err) || filepath.Dir(absolute) == absolute {
		return "", err
	}
	parent, err := canonicalFolder(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}
