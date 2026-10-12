// frontmatter normalizes the content repository without loading renderer config.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/astaxie/beego/logs"
	"github.com/sbraveyoung/gobog/src/article"
)

type change struct {
	Path          string `json:"path"`
	BeforeSHA     string `json:"before_sha256"`
	AfterSHA      string `json:"after_sha256"`
	before, after []byte
	mode          fs.FileMode
}

func main() {
	source := flag.String("source", "", "blog source root")
	author := flag.String("author", "", "default author for missing fields")
	apply := flag.Bool("apply", false, "apply changes; default is dry run")
	backup := flag.String("backup", "", "required empty backup directory when applying")
	report := flag.String("report", "", "optional JSON report")
	flag.Parse()
	if err := run(*source, *author, *apply, *backup, *report); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(source, author string, apply bool, backup, report string) error {
	if source == "" {
		return fmt.Errorf("-source is required")
	}
	root, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	logs.SetLevel(logs.LevelError)
	scanned := 0
	changes := []change{}
	err = filepath.WalkDir(root, func(p string, e fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if p != root && strings.HasPrefix(e.Name(), ".") {
			if e.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.IsDir() {
			return nil
		}
		if e.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(e.Name(), ".md") || e.Name() == "_folder.md" {
			return nil
		}
		slash := filepath.ToSlash(rel)
		if !strings.HasPrefix(slash, "post/") && !(strings.HasPrefix(slash, "pages/") && strings.Count(slash, "/") == 1) {
			return nil
		}
		before, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		id := article.CalcID([]byte(filepath.ToSlash(rel)))
		created, updated := info.ModTime(), info.ModTime()
		parsed, err := article.ParseFile(p)
		if err != nil {
			return err
		}
		if parsed.CreateTime == "" || parsed.UpdatedTime == "" {
			created, updated = gitDates(root, slash, info.ModTime())
		}
		prefix := "/post/"
		parts := strings.Split(strings.TrimSuffix(slash, ".md"), "/")
		for i := 1; i < len(parts); i++ {
			parts[i] = article.SlugOrHash(parts[i])
		}
		url := prefix + strings.Join(parts[1:], "/")
		if parts[0] == "pages" {
			url = "/" + article.SlugOrHash(strings.TrimSuffix(e.Name(), ".md"))
		}
		after, err := article.NormalizeFrontMatter(before, article.NormalizeDefaults{Title: strings.TrimSuffix(e.Name(), ".md"), Author: author, ID: id, URL: url, Time: created, UpdatedTime: updated})
		if err != nil {
			return fmt.Errorf("%s: %w", rel, err)
		}
		scanned++
		if !bytes.Equal(before, after) {
			changes = append(changes, change{Path: rel, BeforeSHA: fmt.Sprintf("%x", sha256.Sum256(before)), AfterSHA: fmt.Sprintf("%x", sha256.Sum256(after)), before: before, after: after, mode: info.Mode().Perm()})
		}
		return nil
	})
	if err != nil {
		return err
	}
	if apply && len(changes) > 0 {
		if backup == "" {
			return fmt.Errorf("-backup is required with -apply")
		}
		b, err := filepath.Abs(backup)
		if err != nil {
			return err
		}
		if b == root || strings.HasPrefix(b, root+string(filepath.Separator)) {
			return fmt.Errorf("backup must be outside blog source")
		}
		if err := os.Mkdir(b, 0700); err != nil {
			return fmt.Errorf("backup must be a new directory: %w", err)
		}
		// Verify every snapshot before writing any article or backup.
		for _, c := range changes {
			current, err := os.ReadFile(filepath.Join(root, c.Path))
			if err != nil {
				return err
			}
			if !bytes.Equal(current, c.before) {
				return fmt.Errorf("%s changed during planning; retry", c.Path)
			}
		}
		for _, c := range changes {
			p := filepath.Join(b, c.Path)
			if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(p, c.before, 0600); err != nil {
				return err
			}
		}
		for _, c := range changes {
			if err := atomicWrite(filepath.Join(root, c.Path), c.after, c.mode); err != nil {
				return err
			}
		}
	}
	result := struct {
		Scanned int      `json:"scanned"`
		Changed int      `json:"changed"`
		Applied bool     `json:"applied"`
		Changes []change `json:"changes"`
	}{scanned, len(changes), apply, changes}
	if report != "" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(report, append(data, '\n'), 0600); err != nil {
			return err
		}
	}
	fmt.Printf("articles=%d changed=%d applied=%v\n", scanned, len(changes), apply)
	return nil
}

func atomicWrite(p string, data []byte, mode fs.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".frontmatter-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), p)
}

// Git history survives checkout/restore operations that change filesystem mtimes.
// Missing dates fall back to mtime only for files without accessible Git history.
func gitDates(root, relative string, fallback time.Time) (time.Time, time.Time) {
	data, err := exec.Command("git", "-C", root, "log", "--follow", "--format=%aI", "--", relative).Output()
	if err != nil {
		return fallback, fallback
	}
	dates := strings.Fields(string(data))
	if len(dates) == 0 {
		return fallback, fallback
	}
	created, e1 := time.Parse(time.RFC3339, dates[len(dates)-1])
	updated, e2 := time.Parse(time.RFC3339, dates[0])
	if e1 != nil || e2 != nil {
		return fallback, fallback
	}
	return created.In(time.Local), updated.In(time.Local)
}
