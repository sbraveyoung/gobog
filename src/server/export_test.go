package server

import (
	"path/filepath"
	"testing"
)

func TestUrlToFile(t *testing.T) {
	out := "/out"
	cases := []struct {
		url, want string
	}{
		{"/", filepath.Join(out, "index.html")},
		{"/post/abc", filepath.Join(out, "post", "abc", "index.html")},
		{"/post/abc/", filepath.Join(out, "post", "abc", "index.html")},
		{"/post/abc/def", filepath.Join(out, "post", "abc", "def", "index.html")},
		{"/about", filepath.Join(out, "about", "index.html")},
	}
	for _, c := range cases {
		got := urlToFile(out, c.url)
		if got != c.want {
			t.Errorf("urlToFile(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}
