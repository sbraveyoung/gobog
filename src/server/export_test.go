package server

import (
	"path/filepath"
	"testing"

	articlepkg "github.com/sbraveyoung/gobog/src/article"
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

func TestWithoutPrivateStripsLeavesAndEmptyGroups(t *testing.T) {
	pub := func(title string) *articlepkg.Article {
		a := &articlepkg.Article{}
		a.Title = title
		return a
	}
	priv := func(title string) *articlepkg.Article {
		a := &articlepkg.Article{}
		a.Title = title
		a.Private = "true"
		return a
	}

	emptyAfterFilter := pub("group-of-secrets")
	emptyAfterFilter.SubArticle = articlepkg.Articles{priv("s1"), priv("s2")}

	mixed := pub("group-mixed")
	mixed.SubArticle = articlepkg.Articles{priv("hidden"), pub("visible")}

	allPublic := pub("group-public")
	allPublic.SubArticle = articlepkg.Articles{pub("a"), pub("b")}

	in := articlepkg.Articles{
		pub("leaf-public"),
		priv("leaf-private"),
		emptyAfterFilter,
		mixed,
		allPublic,
	}

	got := withoutPrivate(in)

	// Want: leaf-public, group-mixed (with one sub), group-public (intact).
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3: %v", len(got), titlesForExport(got))
	}
	titles := titlesForExport(got)
	for _, want := range []string{"leaf-public", "group-mixed", "group-public"} {
		if !contains(titles, want) {
			t.Errorf("missing %q in %v", want, titles)
		}
	}
	for _, dontWant := range []string{"leaf-private", "group-of-secrets"} {
		if contains(titles, dontWant) {
			t.Errorf("private %q leaked into export: %v", dontWant, titles)
		}
	}
}

func titlesForExport(a articlepkg.Articles) []string {
	out := make([]string, len(a))
	for i, x := range a {
		out[i] = x.Title
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
