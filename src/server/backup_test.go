package server

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWriteArchiveSkipsHiddenTopDirs(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "post.md"), "hello")
	writeTestFile(t, filepath.Join(src, "Tech", "http.md"), "world")
	writeTestFile(t, filepath.Join(src, ".obsidian", "config.json"), "secret")
	writeTestFile(t, filepath.Join(src, ".git", "HEAD"), "ref:")

	out := filepath.Join(t.TempDir(), "test.tar.gz")
	if err := writeArchive(src, out); err != nil {
		t.Fatal(err)
	}

	names := tarContents(t, out)
	wantPresent := map[string]bool{"post.md": false, "Tech": false, "Tech/http.md": false}
	for _, n := range names {
		if _, ok := wantPresent[n]; ok {
			wantPresent[n] = true
		}
		if strings.HasPrefix(n, ".obsidian") || strings.HasPrefix(n, ".git") {
			t.Errorf("hidden dir leaked into archive: %q", n)
		}
	}
	for k, seen := range wantPresent {
		if !seen {
			t.Errorf("expected %q in archive, didn't find it. Got: %v", k, names)
		}
	}
}

func tarContents(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var out []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, hdr.Name)
	}
	return out
}

func TestRotateBackupsKeepsNewestN(t *testing.T) {
	dir := t.TempDir()
	// Create 5 archive files with deterministic-but-different timestamps so
	// lex sort matches creation order.
	names := []string{
		"gobog-20260101-100000.tar.gz",
		"gobog-20260102-100000.tar.gz",
		"gobog-20260103-100000.tar.gz",
		"gobog-20260104-100000.tar.gz",
		"gobog-20260105-100000.tar.gz",
	}
	for _, n := range names {
		writeTestFile(t, filepath.Join(dir, n), "x")
	}
	// Also drop in a non-archive file — should be ignored by rotation.
	writeTestFile(t, filepath.Join(dir, "notes.txt"), "x")

	rotateBackups(dir, 2)

	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool)
	for _, e := range left {
		got[e.Name()] = true
	}
	wantKeep := []string{
		"gobog-20260105-100000.tar.gz",
		"gobog-20260104-100000.tar.gz",
		"notes.txt",
	}
	wantGone := []string{
		"gobog-20260101-100000.tar.gz",
		"gobog-20260102-100000.tar.gz",
		"gobog-20260103-100000.tar.gz",
	}
	for _, n := range wantKeep {
		if !got[n] {
			t.Errorf("want %q present, got %v", n, mapKeys(got))
		}
	}
	for _, n := range wantGone {
		if got[n] {
			t.Errorf("want %q removed, still present: %v", n, mapKeys(got))
		}
	}
}

func TestRotateBackupsKeepZeroIsNoOp(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "gobog-20260101-100000.tar.gz"), "x")
	rotateBackups(dir, 0)
	if _, err := os.Stat(filepath.Join(dir, "gobog-20260101-100000.tar.gz")); err != nil {
		t.Errorf("keep=0 should preserve all archives: %v", err)
	}
}

func TestBackupOnceRoundTrip(t *testing.T) {
	src := t.TempDir()
	writeTestFile(t, filepath.Join(src, "a.md"), "hello world")

	dir := t.TempDir()
	if err := backupOnce(src, dir, 7); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("want 1 archive, got %d", len(entries))
	}
	names := tarContents(t, filepath.Join(dir, entries[0].Name()))
	if len(names) == 0 {
		t.Errorf("archive is empty")
	}
}

func TestArchiveNameUsesUTC(t *testing.T) {
	now := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	got := archiveName(now)
	if got != "gobog-20260425-120000.tar.gz" {
		t.Errorf("archiveName = %q", got)
	}
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
