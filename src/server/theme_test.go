package server

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/sbraveyoung/gobog/src/config"
)

// makeThemes creates a fake themes/ tree with the given theme names. Each
// theme is a directory containing an empty index.html (the marker
// availableThemes uses). Returns the path to use as config.C.Blog.Theme.
func makeThemes(t *testing.T, names []string, current string) string {
	t.Helper()
	root := t.TempDir()
	themesDir := filepath.Join(root, "themes")
	if err := os.MkdirAll(themesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range names {
		dir := filepath.Join(themesDir, n)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Add a stray non-theme directory and a file to make sure they're filtered.
	_ = os.MkdirAll(filepath.Join(themesDir, "not-a-theme"), 0o755)
	_ = os.WriteFile(filepath.Join(themesDir, "stray.txt"), []byte("x"), 0o644)
	return filepath.Join(themesDir, current)
}

func TestAvailableThemes(t *testing.T) {
	prev := config.C.Blog.Theme
	defer func() { config.C.Blog.Theme = prev }()

	config.C.Blog.Theme = makeThemes(t, []string{"letter", "minimal", "tufte"}, "minimal")
	got := availableThemes()
	sort.Strings(got)
	want := []string{"letter", "minimal", "tufte"}
	if len(got) != len(want) {
		t.Fatalf("availableThemes len: got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("availableThemes[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestThemeFor(t *testing.T) {
	prev := config.C.Blog.Theme
	defer func() { config.C.Blog.Theme = prev }()
	config.C.Blog.Theme = makeThemes(t, []string{"letter", "minimal"}, "minimal")
	parent := filepath.Dir(config.C.Blog.Theme)

	tests := []struct {
		name   string
		cookie string
		want   string
	}{
		{"no cookie falls back to default", "", config.C.Blog.Theme},
		{"valid theme name resolves to its dir", "letter", filepath.Join(parent, "letter")},
		{"unknown theme falls back to default", "nope", config.C.Blog.Theme},
		{"path traversal rejected", "../etc", config.C.Blog.Theme},
		{"slash rejected", "letter/css", config.C.Blog.Theme},
		{"dot rejected", ".", config.C.Blog.Theme},
		{"empty string falls back", "", config.C.Blog.Theme},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", "/", nil)
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: themeCookie, Value: tc.cookie})
			}
			if got := themeFor(r); got != tc.want {
				t.Errorf("themeFor cookie=%q: got %q want %q", tc.cookie, got, tc.want)
			}
		})
	}
}

func TestThemeForNilRequest(t *testing.T) {
	prev := config.C.Blog.Theme
	defer func() { config.C.Blog.Theme = prev }()
	config.C.Blog.Theme = "themes/minimal"
	if got := themeFor(nil); got != "themes/minimal" {
		t.Errorf("themeFor(nil): got %q want %q", got, "themes/minimal")
	}
}
