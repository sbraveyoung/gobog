package server

import "testing"

func TestCoverURLPassesThroughAbsolute(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"  ", ""},
		{"hero.png", "/image/hero.png"},
		{"sub/dir/hero.jpg", "/image/sub/dir/hero.jpg"},
		{"/image/hero.png", "/image/hero.png"},
		{"/static/hero.png", "/static/hero.png"},
		{"https://cdn.example.com/h.png", "https://cdn.example.com/h.png"},
		{"http://cdn.example.com/h.png", "http://cdn.example.com/h.png"},
	}
	for _, c := range cases {
		if got := coverURL(c.in); got != c.want {
			t.Errorf("coverURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
