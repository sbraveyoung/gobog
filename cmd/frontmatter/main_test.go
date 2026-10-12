package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDryRunApplyBackupAndRepeat(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "source")
	os.MkdirAll(filepath.Join(dir, "post"), 0755)
	p := filepath.Join(dir, "post/note.md")
	before := []byte("---\ntitle: Note\nid: stable\nurl: /post/stable\n---\n# body\n")
	os.WriteFile(p, before, 0644)
	control := filepath.Join(dir, "post/_folder.md")
	os.WriteFile(control, []byte("---\nhidden: false\n---\n"), 0644)
	backup := filepath.Join(root, "backup")
	if err := run(dir, "author", false, "", ""); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("dry run wrote note")
	}
	if err := run(dir, "author", true, backup, ""); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(filepath.Join(backup, "post/note.md"))
	if !bytes.Equal(saved, before) {
		t.Fatal("backup does not match input")
	}
	after, _ = os.ReadFile(p)
	if bytes.Equal(before, after) {
		t.Fatal("apply did not normalize")
	}
	if err := run(dir, "author", true, "", ""); err != nil {
		t.Fatal("idempotent repeat failed:", err)
	}
	again, _ := os.ReadFile(p)
	if !bytes.Equal(after, again) {
		t.Fatal("repeat changed metadata")
	}
	policy, _ := os.ReadFile(control)
	if !bytes.Equal(policy, []byte("---\nhidden: false\n---\n")) {
		t.Fatal("normalizer changed folder policy")
	}
}

func TestInvalidYAMLAbortsBeforeAnyWrite(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "post"), 0755)
	before := []byte("body")
	p := filepath.Join(root, "post/a.md")
	os.WriteFile(p, before, 0644)
	os.WriteFile(filepath.Join(root, "post/z.md"), []byte("---\nbad: [\n---\nbody"), 0644)
	if err := run(root, "author", true, filepath.Join(t.TempDir(), "backup"), ""); err == nil {
		t.Fatal("invalid YAML accepted")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("partial normalization after invalid YAML")
	}
}
