package blog

import (
	"github.com/sbraveyoung/gobog/src/config"
	"path/filepath"
	"testing"
)

func TestReloadRejectsDuplicateAndUnsafeRoutes(t *testing.T) {
	for _, route := range []string{"/post/same", "/../../escape", "/", "/css/stolen", "/post/../about", "https://elsewhere.example/post"} {
		t.Run(route, func(t *testing.T) {
			root := t.TempDir()
			writeNote(t, filepath.Join(root, "post/a.md"), "---\ntitle: A\nurl: "+route+"\n---\nbody\n")
			if route == "/post/same" {
				writeNote(t, filepath.Join(root, "post/b.md"), "---\ntitle: B\nurl: /post/same/\n---\nbody\n")
			}
			prev := config.C.Blog
			config.C.Blog.Source = root
			t.Cleanup(func() { config.C.Blog = prev })
			if err := Blog.Reload(); err == nil {
				t.Error("unsafe/colliding route accepted")
			}
		})
	}
}
