package server

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	ttemplate "text/template"

	articlepkg "github.com/sbraveyoung/gobog/src/article"
	"github.com/sbraveyoung/gobog/src/config"
	"github.com/astaxie/beego/logs"
)

// Snippet sharing — gist-style. Each snippet is a tiny markdown blob (or
// fenced code) stored as a flat file under <data>/snippets/<id>.md. POST
// /snippet (auth required) creates one and returns the URL; GET
// /snippet/<id> renders it through the same article pipeline so syntax
// highlighting, theming etc. all just work.
//
// Anonymous creation is intentionally NOT supported — you need [auth] set
// in config and Basic auth on the POST. Without that the endpoint is 503.

const (
	snippetMaxBytes  = 256 * 1024 // 256 KiB
	snippetIDPattern = `^[a-zA-Z0-9_-]{6,32}$`
)

var snippetIDRE = regexp.MustCompile(snippetIDPattern)

// snippetDir resolves the on-disk directory for snippets, creating it on
// demand so the first POST works without any setup.
func snippetDir() (string, error) {
	dir := config.C.Data.Dir
	if dir == "" {
		dir = "./gobog-data"
	}
	out := filepath.Join(dir, "snippets")
	return out, os.MkdirAll(out, 0o755)
}

// snippetHandler dispatches GET and POST on /snippet/[id].
func snippetHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		// /snippet (creation) — id is generated, body is the snippet content.
		if r.URL.Path != "/snippet" && r.URL.Path != "/snippet/" {
			http.Error(w, "POST goes to /snippet (no id)", http.StatusBadRequest)
			return
		}
		if !requireAuth(w, r) {
			return
		}
		createSnippet(w, r)
	case http.MethodGet:
		id := strings.TrimPrefix(r.URL.Path, "/snippet")
		id = strings.Trim(id, "/")
		if id == "" {
			renderSnippetForm(w, r)
			return
		}
		showSnippet(w, r, id)
	default:
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func createSnippet(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, snippetMaxBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "request too large or malformed", http.StatusBadRequest)
		return
	}
	body := r.Form.Get("body")
	if body == "" {
		http.Error(w, "body is empty", http.StatusBadRequest)
		return
	}

	id, err := newSnippetID()
	if err != nil {
		http.Error(w, "id gen failed", http.StatusInternalServerError)
		logs.Error("snippet id gen:", err)
		return
	}
	dir, err := snippetDir()
	if err != nil {
		http.Error(w, "snippet dir not writable", http.StatusInternalServerError)
		logs.Error("snippet dir:", err)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, id+".md"), []byte(body), 0o644); err != nil {
		http.Error(w, "snippet write failed", http.StatusInternalServerError)
		logs.Error("snippet write:", err)
		return
	}

	url := "/snippet/" + id
	// Honor Accept: application/json for programmatic clients (curl scripts).
	if strings.Contains(r.Header.Get("Accept"), "application/json") {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":%q,"url":%q}`, id, url)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

func showSnippet(w http.ResponseWriter, r *http.Request, id string) {
	if !snippetIDRE.MatchString(id) {
		notFound(w, r)
		return
	}
	dir, err := snippetDir()
	if err != nil {
		http.Error(w, "snippet dir not readable", http.StatusInternalServerError)
		return
	}
	body, err := os.ReadFile(filepath.Join(dir, id+".md"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			notFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Render through the existing article pipeline so syntax highlighting,
	// canonical tags, and the standard theme template all kick in.
	a := &articlepkg.Article{}
	a.Title = "Snippet " + id
	a.URL = "/snippet/" + id
	a.Content = body
	rendered, err := renderArticleHTML(a)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, err := ttemplate.ParseFiles(themeFor(r) + "/post.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	view := newArticleView(a, rendered, config.C.Blog.Domain)
	if err := t.Execute(w, view); err != nil {
		logs.Warn("snippet render:", err)
	}
}

func renderSnippetForm(w http.ResponseWriter, r *http.Request) {
	const form = `<!doctype html><html><head><meta charset="utf-8"><title>New snippet</title>
<style>body{font:14px/1.4 monospace;max-width:720px;margin:2em auto;padding:0 1em}
textarea{width:100%;min-height:18em;font:inherit}</style></head><body>
<h1>New snippet</h1>
<p>POST a snippet body to <code>/snippet</code> (HTTP Basic auth required).
The response is a 303 redirect to the rendered URL, or a JSON body when the
request asks for <code>application/json</code>.</p>
<p>Quick test from the browser:</p>
<form method="POST" action="/snippet">
  <textarea name="body" placeholder="` + "```go\nfmt.Println(\"hi\")\n```" + `"></textarea>
  <p><button type="submit">Create</button></p>
</form>
</body></html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(form))
}

func newSnippetID() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// base64url then strip padding; six random bytes -> eight URL-safe chars.
	return strings.TrimRight(base64.URLEncoding.EncodeToString(buf), "="), nil
}
