package server

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	ttemplate "text/template"

	articlepkg "github.com/SmartBrave/gobog/src/article"
	"github.com/SmartBrave/gobog/src/blog"
	"github.com/SmartBrave/gobog/src/config"
	httpc "github.com/SmartBrave/utils/easyhttpclient"
	"github.com/astaxie/beego/logs"
	"github.com/facebookarchive/grace/gracehttp"
)

type Server struct{}

var server = &Server{}

// Run is the entrypoint for the HTTP servers. It blocks. Used by main.go when
// not running in static-export mode.
func Run() error {
	initViewStore()
	startBackupWorker(make(chan struct{})) // never stopped; gracehttp owns the lifecycle

	// Try to load the TLS keypair upfront. Missing or unreadable files
	// downgrade to a warning so the operator can still run plain HTTP for
	// dev/local setups without scrubbing [http].cert/key from the config.
	var tlsCert *tls.Certificate
	if config.C.Http.Cert != "" && config.C.Http.Key != "" {
		c, err := tls.LoadX509KeyPair(config.C.Http.Cert, config.C.Http.Key)
		if err != nil {
			logs.Warn("TLS keypair could not be loaded, continuing without HTTPS:", err)
		} else {
			tlsCert = &c
		}
	}

	servers := []*http.Server{}

	// Plain HTTP: only act as a redirector when TLS actually loaded;
	// otherwise serve the site directly so [http].redirect_tls=true with
	// a missing cert doesn't bounce users into a void.
	if config.C.Http.Addr != "" {
		var handler http.Handler
		if config.C.Http.RedirectTLS && tlsCert != nil && config.C.Http.Addrs != "" {
			handler = http.HandlerFunc(redirectToHTTPS)
		} else {
			handler = server.newHandler()
		}
		servers = append(servers, &http.Server{
			Addr:    config.C.Http.Addr,
			Handler: handler,
		})
	}

	if config.C.Http.Addrs != "" && tlsCert != nil {
		servers = append(servers, &http.Server{
			Addr:    config.C.Http.Addrs,
			Handler: server.newHandler(),
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{*tlsCert},
			},
		})
	}

	if len(servers) == 0 {
		return fmt.Errorf("no listeners configured: set [http].addr and/or [http].addrs")
	}

	if err := gracehttp.Serve(servers...); err != nil {
		return fmt.Errorf("gracehttp.Serve: %w", err)
	}
	return nil
}

func redirectToHTTPS(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	addrs := strings.TrimPrefix(config.C.Http.Addrs, ":")
	target := "https://" + host
	if addrs != "" && addrs != "443" {
		target += ":" + addrs
	}
	target += r.URL.RequestURI()
	http.Redirect(w, r, target, http.StatusMovedPermanently)
}

func (s *Server) newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", logMiddle(rootHandler))
	mux.HandleFunc("/post/", logMiddle(postHandler))
	mux.HandleFunc("/about", logMiddle(aboutHandler))
	mux.HandleFunc("/tag/", logMiddle(tagHandler))
	mux.HandleFunc("/search", logMiddle(searchHandler))
	mux.HandleFunc("/atom.xml", logMiddle(atomHandler))
	mux.HandleFunc("/sitemap.xml", logMiddle(sitemapHandler))
	mux.HandleFunc("/robots.txt", logMiddle(robotsHandler))
	mux.HandleFunc("/healthz", healthzHandler)
	mux.HandleFunc("/image/", logMiddle(imageHandler))
	mux.HandleFunc("/css/", logMiddle(cssHandler))
	mux.HandleFunc("/js/", logMiddle(jsHandler))
	mux.HandleFunc("/bing_img", logMiddle(bingImgHandler))
	mux.HandleFunc("/snippet", logMiddle(snippetHandler))
	mux.HandleFunc("/snippet/", logMiddle(snippetHandler))
	return mux
}

func logMiddle(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		logs.Info(fmt.Sprintf("%s %s", r.Method, r.URL))
		f(w, r)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		notFound(w, r)
		return
	}
	t, err := template.ParseFiles(config.C.Blog.Theme + "/index.html")
	if err != nil {
		logs.Error("parse index template:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := t.Execute(w, blog.Blog.Groups()); err != nil {
		logs.Error("exec index template:", err)
	}
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path
	matched := findArticle(blog.Blog.Groups(), urlPath)
	if matched == nil {
		// Diagnostic: dump up to 5 known URLs so an operator hitting an
		// unexpected 404 can see what the scanner actually has indexed.
		known := knownPostURLs(5)
		logs.Warn(fmt.Sprintf("post 404: requested=%q, %d posts indexed, sample=%v", urlPath, len(blog.Blog.AllPosts()), known))
		notFound(w, r)
		return
	}
	if len(matched.SubArticle) == 0 {
		renderPost(w, r, matched)
	} else {
		renderGroup(w, matched.SubArticle)
	}
}

// knownPostURLs returns up to n leaf-post URLs from the index — used to
// surface diagnostics on /post/ 404s.
func knownPostURLs(n int) []string {
	all := blog.Blog.AllPosts()
	if len(all) > n {
		all = all[:n]
	}
	out := make([]string, 0, len(all))
	for _, a := range all {
		out = append(out, a.URL)
	}
	return out
}

// findArticle walks the article tree (any depth) looking for an exact URL
// match. The earlier version short-circuited recursion unless the requested
// path started with the group's URL — but that breaks when an article's
// front-matter pins a URL that doesn't share its parent group's slug
// (typical with legacy posts whose `url:` was rewritten to `/post/<hex>/<hex>`
// but whose physical location is now nested deeper). Walk every node.
func findArticle(list articlepkg.Articles, urlPath string) *articlepkg.Article {
	for _, a := range list {
		if a.URL == urlPath {
			return a
		}
		if len(a.SubArticle) > 0 {
			if hit := findArticle(a.SubArticle, urlPath); hit != nil {
				return hit
			}
		}
	}
	return nil
}

func renderPost(w http.ResponseWriter, r *http.Request, article *articlepkg.Article) {
	// Private articles gate the body behind HTTP Basic auth, but only if
	// auth is configured. Without [auth], private notes are inaccessible
	// (503) rather than silently public.
	if article.IsPrivate() && !requireAuth(w, r) {
		return
	}

	parse, err := renderArticleHTML(article)
	if err != nil {
		logs.Warn("render markdown:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	t, err := ttemplate.ParseFiles(config.C.Blog.Theme + "/post.html")
	if err != nil {
		logs.Warn("parse post template:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if views != nil {
		views.Increment(article.URL)
	}
	view := newArticleView(article, parse, config.C.Blog.Domain)
	if err := t.Execute(w, view); err != nil {
		logs.Warn("exec post template:", err)
	}
}

func renderGroup(w http.ResponseWriter, list articlepkg.Articles) {
	t, err := template.ParseFiles(config.C.Blog.Theme + "/index.html")
	if err != nil {
		logs.Warn("parse group template:", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := t.Execute(w, list); err != nil {
		logs.Warn("exec group template:", err)
	}
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	list := blog.Blog.Articles(blog.BlogTypes["about"])
	if len(list) == 0 {
		notFound(w, r)
		return
	}
	renderPost(w, r, list[0])
}

func tagHandler(w http.ResponseWriter, r *http.Request) {
	tag := strings.TrimPrefix(r.URL.Path, "/tag/")
	tag = strings.Trim(tag, "/")
	if tag == "" {
		// All-tags index page: render the post template using a synthetic
		// article whose body lists every tag.
		var sb strings.Builder
		sb.WriteString("<ul>")
		for _, t := range blog.Blog.Tags() {
			fmt.Fprintf(&sb, "<li><a href=\"/tag/%s\">%s</a></li>", t, t)
		}
		sb.WriteString("</ul>")
		writeSyntheticPost(w, "Tags", sb.String())
		return
	}
	posts := blog.Blog.PostsByTag(tag)
	if len(posts) == 0 {
		notFound(w, r)
		return
	}
	renderGroup(w, posts)
}

func searchHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("query"))
	if q == "" {
		writeSyntheticPost(w, "Search", `<form method="get" action="/search"><input name="query" placeholder="search" autofocus/><button>Go</button></form>`)
		return
	}
	results := searchPosts(q)
	if len(results) == 0 {
		writeSyntheticPost(w, "Search: "+q, "<p>No matching posts.</p>")
		return
	}
	renderGroup(w, results)
}

func searchPosts(query string) articlepkg.Articles {
	q := strings.ToLower(query)
	type scored struct {
		a     *articlepkg.Article
		score int
	}
	var hits []scored
	for _, a := range blog.Blog.AllPosts() {
		score := 0
		title := strings.ToLower(a.Title)
		if strings.Contains(title, q) {
			score += 10
		}
		for _, t := range a.Tags {
			if strings.Contains(strings.ToLower(t), q) {
				score += 5
			}
		}
		if strings.Contains(strings.ToLower(string(a.Content)), q) {
			score += 1
		}
		if score > 0 {
			hits = append(hits, scored{a, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := make(articlepkg.Articles, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.a)
	}
	return out
}

func writeSyntheticPost(w http.ResponseWriter, title, bodyHTML string) {
	t, err := ttemplate.ParseFiles(config.C.Blog.Theme + "/post.html")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	a := &articlepkg.Article{}
	a.Title = title
	a.URL = "/"
	view := newArticleView(a, "<h2>"+title+"</h2>"+bodyHTML, config.C.Blog.Domain)
	_ = t.Execute(w, view)
}

func notFound(w http.ResponseWriter, r *http.Request) {
	logs.Warn("404:", r.URL.Path)
	t, err := ttemplate.ParseFiles(config.C.Blog.Theme + "/post.html")
	if err == nil {
		w.WriteHeader(http.StatusNotFound)
		a := &articlepkg.Article{}
		a.Title = "Not Found"
		a.URL = r.URL.Path
		view := newArticleView(a, "<h2>404 Not Found</h2><p>Path "+template.HTMLEscapeString(r.URL.Path)+" does not exist.</p><p><a href=\"/\">Home</a></p>", config.C.Blog.Domain)
		_ = t.Execute(w, view)
		return
	}
	http.NotFound(w, r)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("ok"))
}

// safeServeFile serves <root>/<r.URL.Path> after verifying that the cleaned
// URL still starts with the given prefix and that the resulting filesystem
// path stays within root. Defends against /image/../../etc/passwd-style
// traversal: even though path.Clean would happily fold the .. to /etc/passwd
// (still under root in absolute terms if root is shallow), we additionally
// require the cleaned URL path to retain its /image/ scope.
func safeServeFile(w http.ResponseWriter, r *http.Request, root, prefix string) {
	cleaned := path.Clean("/" + r.URL.Path)
	if cleaned != strings.TrimRight(prefix, "/") && !strings.HasPrefix(cleaned, prefix) {
		http.NotFound(w, r)
		return
	}
	full := filepath.Join(root, filepath.FromSlash(cleaned))
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	fullAbs, err := filepath.Abs(full)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !strings.HasPrefix(fullAbs, rootAbs+string(filepath.Separator)) && fullAbs != rootAbs {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, full)
}

func imageHandler(w http.ResponseWriter, r *http.Request) {
	// First try the legacy <source>/image/<rest> path. If the file isn't
	// there, fall back to the vault image index by basename, which lets
	// Obsidian users keep images alongside their notes (or in any
	// attachments folder) instead of forcing them into a single dir.
	cleaned := path.Clean("/" + r.URL.Path)
	if !strings.HasPrefix(cleaned, "/image/") {
		http.NotFound(w, r)
		return
	}

	rootAbs, _ := filepath.Abs(config.C.Blog.Source)

	// Resolve to a filesystem path: legacy <source>/image/X first, vault
	// index by basename second.
	var fullAbs string
	legacyAbs, _ := filepath.Abs(filepath.Join(config.C.Blog.Source, filepath.FromSlash(cleaned)))
	if legacyAbs != "" && strings.HasPrefix(legacyAbs, rootAbs+string(filepath.Separator)) {
		if fi, err := os.Stat(legacyAbs); err == nil && !fi.IsDir() {
			fullAbs = legacyAbs
		}
	}
	if fullAbs == "" {
		base := path.Base(cleaned)
		if rel := blog.Blog.Wiki().ResolveImage(base); rel != "" {
			full := filepath.Join(config.C.Blog.Source, filepath.FromSlash(rel))
			abs, _ := filepath.Abs(full)
			if abs != "" && strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) {
				fullAbs = abs
			}
		}
	}
	if fullAbs == "" {
		http.NotFound(w, r)
		return
	}

	// Watermark JPEG/PNG when configured. serveWatermarked falls back
	// internally if the format isn't supported or decoding fails.
	if serveWatermarked(w, r, fullAbs) {
		return
	}
	http.ServeFile(w, r, fullAbs)
}

func cssHandler(w http.ResponseWriter, r *http.Request) {
	safeServeFile(w, r, config.C.Blog.Theme, "/css/")
}

func jsHandler(w http.ResponseWriter, r *http.Request) {
	safeServeFile(w, r, config.C.Blog.Theme, "/js/")
}

func bingImgHandler(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if err := recover(); err != nil {
			logs.Error("panic in bingImgHandler:", err)
		}
	}()
	const url = "https://cn.bing.com/HPImageArchive.aspx?format=js&idx=0&n=1"

	client := httpc.NewHttpClient(url).M("GET")
	respCode, data, err := client.Do()
	if err != nil || respCode != 200 {
		logs.Error("bing api:", err)
		w.Write([]byte("Server Error"))
		return
	}
	var j resp
	if err := data.Unmarshal(&j); err != nil {
		logs.Error("bing decode:", err)
		w.Write([]byte("Server Error"))
		return
	}
	if len(j.Images) == 0 {
		w.Write([]byte("Server Error"))
		return
	}
	_, image, err := httpc.NewHttpClient("https://cn.bing.com" + j.Images[0].Url).M("GET").Do()
	if err != nil {
		logs.Error("bing fetch:", err)
		w.Write([]byte("Server Error"))
		return
	}
	w.Write(image.Data)
}

