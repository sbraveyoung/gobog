package server

import (
	"strings"
	"testing"

	"github.com/sbraveyoung/gobog/src/blog"
)

func TestExpandWikilinks(t *testing.T) {
	wiki := blog.NewWikiIndexForTesting(
		map[string]string{
			"http":            "/post/tech/http",
			"tech/networking": "/post/tech/networking",
			"about me":        "/about",
		},
		map[string]string{
			"diagram.png": "Tech/Networking/diagram.png",
		},
	)

	cases := []struct {
		name string
		in   string
		want []string // substrings the result must contain
		miss []string // substrings the result must NOT contain
	}{
		{
			name: "simple wikilink resolves",
			in:   "see [[HTTP]] for details",
			want: []string{"[HTTP](/post/tech/http)"},
			miss: []string{"[[HTTP]]"},
		},
		{
			name: "wikilink with display text",
			in:   "read the [[HTTP|protocol guide]] now",
			want: []string{"[protocol guide](/post/tech/http)"},
		},
		{
			name: "wikilink with section anchor",
			in:   "jump to [[HTTP#headers]]",
			want: []string{"(/post/tech/http#headers)"},
		},
		{
			name: "path-style wikilink",
			in:   "see [[Tech/Networking]]",
			want: []string{"[Tech/Networking](/post/tech/networking)"},
		},
		{
			name: "unresolved wikilink degrades to plain text",
			in:   "missing [[Nonexistent Note]]",
			want: []string{"missing Nonexistent Note"},
			miss: []string{"[[Nonexistent Note]]", "<a"},
		},
		{
			name: "image embed",
			in:   "![[diagram.png]]",
			want: []string{`<img src="/image/Tech/Networking/diagram.png"`},
		},
		{
			name: "image embed with alt text",
			in:   "![[diagram.png|architecture overview]]",
			want: []string{`alt="architecture overview"`},
		},
		{
			name: "unknown image is dropped to alt text",
			in:   "![[missing.png|fallback text]]",
			want: []string{"fallback text"},
			miss: []string{"<img"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(expandWikilinks([]byte(c.in), wiki))
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q\nfull output: %s", w, got)
				}
			}
			for _, m := range c.miss {
				if strings.Contains(got, m) {
					t.Errorf("output should not contain %q\nfull output: %s", m, got)
				}
			}
		})
	}
}

// TestProtectMath ensures `$$...$$` blocks survive goldmark unchanged so
// MathJax sees the original `\begin{matrix} ... \\ ... \end{matrix}` form.
// Without protection goldmark eats one of the row-separating `\\`.
func TestProtectMath(t *testing.T) {
	body := []byte("Before\n\n$$\n\\begin{matrix}\n1 & 2 \\\\\n3 & 4\n\\end{matrix}\n$$\n\nAfter\n")
	protected, stash := protectMath(body)
	if strings.Contains(string(protected), "\\begin{matrix}") {
		t.Fatalf("protectMath did not stash the math block: %s", protected)
	}
	if len(stash) != 1 {
		t.Errorf("expected 1 math block, got %d", len(stash))
	}
	restored := restoreMath(string(protected), stash)
	if !strings.Contains(restored, "1 & 2 \\\\") {
		t.Errorf("restoreMath lost the row-separating backslashes: %s", restored)
	}
	if !strings.Contains(restored, "\\end{matrix}") {
		t.Errorf("restoreMath lost the end delimiter: %s", restored)
	}
}
