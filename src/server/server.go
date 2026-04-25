package server

import (
	"crypto/tls"
	"fmt"
	"html/template"
	"net/http"
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
	servers := []*http.Server{}

	// Plain HTTP: redirect to HTTPS when TLS is configured and the operator
	// asked for it; otherwise share the same handler with the TLS server.
	if config.C.Http.Addr != "" {
		var handler http.Handler
		if config.C.Http.RedirectTLS && config.C.Http.Cert != "" && config.C.Http.Key != "" {
			handler = http.HandlerFunc(redirectToHTTPS)
		} else {
			handler = server.newHandler()
		}
		servers = append(servers, &http.Server{
			Addr:    config.C.Http.Addr,
			Handler: handler,
		})
	}

	if config.C.Http.Addrs != "" && config.C.Http.Cert != "" && config.C.Http.Key != "" {
		certificate, err := tls.LoadX509KeyPair(config.C.Http.Cert, config.C.Http.Key)
		if err != nil {
			return fmt.Errorf("load TLS keypair: %w", err)
		}
		servers = append(servers, &http.Server{
			Addr:    config.C.Http.Addrs,
			Handler: server.newHandler(),
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{certificate},
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
	groups := blog.Blog.Groups()

	var matched *articlepkg.Article
	for _, g := range groups {
		if !strings.HasPrefix(urlPath, g.URL) {
			continue
		}
		matched = g
		// If the URL points deeper than the group, walk into sub-articles.
		if urlPath != g.URL && len(g.SubArticle) > 0 {
			for _, sub := range g.SubArticle {
				if urlPath == sub.URL {
					matched = sub
					break
				}
			}
		}
		break
	}

	if matched == nil {
		notFound(w, r)
		return
	}

	if len(matched.SubArticle) == 0 {
		renderPost(w, matched)
	} else {
		renderGroup(w, matched.SubArticle)
	}
}

func renderPost(w http.ResponseWriter, article *articlepkg.Article) {
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
	renderPost(w, list[0])
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
	safeServeFile(w, r, config.C.Blog.Source, "/image/")
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

