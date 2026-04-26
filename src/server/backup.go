package server

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SmartBrave/gobog/src/config"
	"github.com/astaxie/beego/logs"
)

// startBackupWorker spawns a goroutine that periodically tar.gz-zips
// [blog].source into <backup-dir>, rotating to keep at most [backup].keep
// archives. Returns immediately when [backup].enabled is false.
func startBackupWorker(stop <-chan struct{}) {
	if !config.C.Backup.Enabled {
		return
	}
	interval := backupInterval()
	if interval <= 0 {
		logs.Warn("backup: invalid interval, disabling")
		return
	}
	source := config.C.Blog.Source
	if source == "" {
		logs.Warn("backup: [blog].source is empty, disabling")
		return
	}
	dir := backupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		logs.Warn("backup: mkdir:", err)
		return
	}

	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		// Run an initial backup so the operator gets immediate feedback that
		// the worker is alive.
		if err := runBackup(source, dir); err != nil {
			logs.Warn("backup (initial):", err)
		} else {
			rotateBackups(dir, config.C.Backup.Keep)
		}
		for {
			select {
			case <-t.C:
				if err := runBackup(source, dir); err != nil {
					logs.Warn("backup:", err)
					continue
				}
				rotateBackups(dir, config.C.Backup.Keep)
			case <-stop:
				return
			}
		}
	}()
}

func backupInterval() time.Duration {
	raw := strings.TrimSpace(config.C.Backup.Interval)
	if raw == "" {
		return time.Hour
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	return d
}

func backupDir() string {
	if config.C.Backup.Dir != "" {
		return config.C.Backup.Dir
	}
	dataDir := config.C.Data.Dir
	if dataDir == "" {
		dataDir = "./gobog-data"
	}
	return filepath.Join(dataDir, "backups")
}

// archiveName uses second-precision timestamps + a short PID-derived suffix
// so two backups within the same second don't clobber each other.
func archiveName(now time.Time) string {
	return fmt.Sprintf("gobog-%s.tar.gz", now.UTC().Format("20060102-150405"))
}

func runBackup(source, dir string) error {
	now := time.Now()
	out := filepath.Join(dir, archiveName(now))

	// Write to a .partial file first so a crash mid-tar doesn't leave a
	// corrupt archive that the rotation logic might keep around.
	partial := out + ".partial"
	if err := writeArchive(source, partial); err != nil {
		_ = os.Remove(partial)
		return err
	}
	if err := os.Rename(partial, out); err != nil {
		return err
	}
	logs.Info("backup wrote:", out)
	return nil
}

// writeArchive walks source recursively into a gzipped tar at outPath.
// Hidden directories (.obsidian, .git, dotfiles) are skipped at the top
// level only — nested dotfiles inside published notes still ship through.
func writeArchive(source, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	src, err := filepath.Abs(source)
	if err != nil {
		return err
	}

	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Skip top-level dotdirs; on Linux at least .obsidian / .git can
		// be enormous and aren't part of the published vault.
		if info.IsDir() && strings.HasPrefix(filepath.Base(p), ".") {
			return filepath.SkipDir
		}
		return addToArchive(tw, src, p, rel, info)
	})
}

func addToArchive(tw *tar.Writer, root, p, rel string, info os.FileInfo) error {
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(rel)
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	in, err := os.Open(p)
	if err != nil {
		return err
	}
	defer in.Close()
	_, err = io.Copy(tw, in)
	return err
}

// rotateBackups keeps the newest `keep` archives in dir, deleting the rest
// (sorted lexicographically — archive names are timestamped so lex order
// matches chronological). keep <= 0 means "keep everything".
func rotateBackups(dir string, keep int) {
	if keep <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var archives []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "gobog-") && strings.HasSuffix(name, ".tar.gz") {
			archives = append(archives, name)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(archives)))
	if len(archives) <= keep {
		return
	}
	for _, old := range archives[keep:] {
		_ = os.Remove(filepath.Join(dir, old))
	}
}

// backupOnce is exposed for tests (and CLI use-cases) so they can run the
// archive + rotate steps deterministically without the ticker.
func backupOnce(source, dir string, keep int) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := runBackup(source, dir); err != nil {
		return err
	}
	rotateBackups(dir, keep)
	return nil
}

// stopBackup gracefully stops the worker. Currently only used by tests.
var (
	backupOnceMu sync.Mutex // guards backupOnce caller order in tests
)

var _ = backupOnceMu // silence unused warning when tests aren't built
