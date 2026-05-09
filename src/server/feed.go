package server

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	articlepkg "github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/blog"
	"github.com/sbraveyoung/gobog/src/config"
)

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Xmlns   string      `xml:"xmlns,attr"`
	Title   string      `xml:"title"`
	Link    []atomLink  `xml:"link"`
	Updated string      `xml:"updated"`
	ID      string      `xml:"id"`
	Author  *atomAuthor `xml:"author,omitempty"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
	Href string `xml:"href,attr"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomEntry struct {
	Title     string      `xml:"title"`
	Link      atomLink    `xml:"link"`
	ID        string      `xml:"id"`
	Updated   string      `xml:"updated"`
	Published string      `xml:"published"`
	Summary   string      `xml:"summary,omitempty"`
	Content   *atomBody   `xml:"content,omitempty"`
	Author    *atomAuthor `xml:"author,omitempty"`
	Category  []atomCat   `xml:"category,omitempty"`
}

type atomBody struct {
	Type string `xml:"type,attr"`
	Body string `xml:",chardata"`
}

type atomCat struct {
	Term string `xml:"term,attr"`
}

func atomHandler(w http.ResponseWriter, r *http.Request) {
	feed := buildAtomFeed()
	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(feed); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func buildAtomFeed() atomFeed {
	domain := strings.TrimRight(config.C.Blog.Domain, "/")
	posts := blog.Blog.AllPosts()
	updated := blog.Blog.LatestUpdate()
	if updated.IsZero() {
		updated = time.Now()
	}

	feed := atomFeed{
		Xmlns:   "http://www.w3.org/2005/Atom",
		Title:   config.C.Blog.Title,
		ID:      domain + "/",
		Updated: updated.UTC().Format(time.RFC3339),
		Link: []atomLink{
			{Rel: "alternate", Type: "text/html", Href: domain + "/"},
			{Rel: "self", Type: "application/atom+xml", Href: domain + "/atom.xml"},
		},
		Author: &atomAuthor{Name: config.C.Blog.Author},
	}

	for _, a := range posts {
		if a.IsPrivate() {
			// Private articles never reach the feed — subscribers shouldn't
			// see titles + summaries that the site itself hides behind
			// auth.
			continue
		}
		t, err := time.Parse(articlepkg.TIME_LAYOUT, a.CreateTime)
		if err != nil {
			t = time.Now()
		}
		entry := atomEntry{
			Title:     a.Title,
			Link:      atomLink{Rel: "alternate", Type: "text/html", Href: domain + a.URL},
			ID:        domain + a.URL,
			Updated:   t.UTC().Format(time.RFC3339),
			Published: t.UTC().Format(time.RFC3339),
			Summary:   a.Summary,
		}
		if a.Author != "" {
			entry.Author = &atomAuthor{Name: a.Author}
		}
		for _, tag := range a.Tags {
			entry.Category = append(entry.Category, atomCat{Term: tag})
		}
		feed.Entries = append(feed.Entries, entry)
	}
	return feed
}

type sitemapURL struct {
	XMLName xml.Name `xml:"url"`
	Loc     string   `xml:"loc"`
	LastMod string   `xml:"lastmod,omitempty"`
}

type sitemapSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

func sitemapHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(buildSitemap()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func buildSitemap() sitemapSet {
	domain := strings.TrimRight(config.C.Blog.Domain, "/")
	urls := []sitemapURL{{Loc: domain + "/"}, {Loc: domain + "/about"}}
	for _, a := range blog.Blog.AllPosts() {
		if a.IsPrivate() {
			continue
		}
		var lastmod string
		if t, err := time.Parse(articlepkg.TIME_LAYOUT, a.CreateTime); err == nil {
			lastmod = t.UTC().Format("2006-01-02")
		}
		urls = append(urls, sitemapURL{Loc: domain + a.URL, LastMod: lastmod})
	}
	for _, t := range blog.Blog.Tags() {
		urls = append(urls, sitemapURL{Loc: domain + "/tag/" + t})
	}
	return sitemapSet{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", URLs: urls}
}

func robotsHandler(w http.ResponseWriter, r *http.Request) {
	domain := strings.TrimRight(config.C.Blog.Domain, "/")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", domain)
}
