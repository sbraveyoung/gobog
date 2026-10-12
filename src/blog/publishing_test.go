package blog

import (
	"github.com/sbraveyoung/gobog/src/config"
	"path/filepath"
	"testing"
)

func TestPublicationSwitches(t *testing.T) {
	for _, tc := range []struct {
		meta     string
		included bool
	}{
		{"publish: false", false}, {"publish: \"false\"", false}, {"publish: true", true}, {"publish: true\ndraft: true", false}, {"draft: \"true\"", false}, {"hidden: true # offline", false}, {"", true},
	} {
		t.Run(tc.meta, func(t *testing.T) {
			root := t.TempDir()
			p := filepath.Join(root, "note.md")
			writeNote(t, p, "---\ntitle: Example\n"+tc.meta+"\n---\nBody\n")
			prev := config.C.Blog
			config.C.Blog.IncludeDrafts = false
			config.C.Blog.IncludeHidden = false
			t.Cleanup(func() { config.C.Blog = prev })
			a, err := buildArticle(p, root, "/post", "note.md")
			if err != nil {
				t.Fatal(err)
			}
			if (a != nil) != tc.included {
				t.Fatalf("included=%v want %v", a != nil, tc.included)
			}
		})
	}
}

func TestPrivateArticleIsOmittedFromStaticScan(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "private.md")
	writeNote(t, p, "---\ntitle: Private\nprivate: true\n---\nSecret\n")
	prev := config.ExportDir
	config.ExportDir = "export"
	t.Cleanup(func() { config.ExportDir = prev })
	a, err := buildArticle(p, root, "/post", "private.md")
	if err != nil {
		t.Fatal(err)
	}
	if a != nil {
		t.Fatal("private article remains in static navigation/wiki index")
	}
}
