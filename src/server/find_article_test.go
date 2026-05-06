package server

import (
	"testing"

	articlepkg "github.com/sbraveyoung/gobog/src/article"
)

// TestFindArticleHonoursFrontMatterURL exercises the regression behind the
// "every post 404s" report: when an article's front-matter pins a URL
// (e.g. /post/<hex>/<hex>) that doesn't share its containing group's slug,
// the lookup must still find it. The previous implementation skipped such
// articles because it required urlPath to be prefixed by the group URL.
func TestFindArticleHonoursFrontMatterURL(t *testing.T) {
	leaf := &articlepkg.Article{}
	leaf.Title = "leaf"
	leaf.URL = "/post/31477598/7d896b00" // legacy-style URL, written by an old NewArticle into front-matter
	group := &articlepkg.Article{SubArticle: articlepkg.Articles{leaf}}
	group.Title = "draft"
	group.URL = "/post/source/draft" // physical-path-derived slug — does NOT prefix the leaf URL

	got := findArticle(articlepkg.Articles{group}, "/post/31477598/7d896b00")
	if got != leaf {
		t.Fatalf("findArticle didn't reach the leaf with a non-prefix URL; got %+v", got)
	}
}

func TestFindArticleStillReturnsNilForUnknown(t *testing.T) {
	leaf := &articlepkg.Article{}
	leaf.URL = "/post/known"
	if got := findArticle(articlepkg.Articles{leaf}, "/post/unknown"); got != nil {
		t.Errorf("expected nil for unknown URL, got %v", got)
	}
}

func TestFindArticleExactMatchOverPrefix(t *testing.T) {
	groupOnly := &articlepkg.Article{}
	groupOnly.URL = "/post/group"
	leaf := &articlepkg.Article{}
	leaf.URL = "/post/group/sub"
	groupOnly.SubArticle = articlepkg.Articles{leaf}

	if got := findArticle(articlepkg.Articles{groupOnly}, "/post/group"); got != groupOnly {
		t.Errorf("group URL should match the group itself, not a sub")
	}
	if got := findArticle(articlepkg.Articles{groupOnly}, "/post/group/sub"); got != leaf {
		t.Errorf("expected leaf match, got %v", got)
	}
}
