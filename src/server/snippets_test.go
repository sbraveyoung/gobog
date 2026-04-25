package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SmartBrave/gobog/src/config"
)

// Snippet handlers depend on config (data dir + auth + theme) and on the
// blog package's wikilink renderer. The renderer just needs an empty index
// to function, so the test setup is small.
func setupSnippetTest(t *testing.T) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	prevData := config.C.Data
	prevAuth := config.C.Auth
	prevBlog := config.C.Blog
	config.C.Data = config.DataConfig{Dir: tmp}
	config.C.Auth = config.AuthConfig{
		Username:     "alice",
		PasswordHash: hashPassword("pw"),
	}
	// Use the actual theme so the post template can be parsed for show().
	config.C.Blog.Theme = filepath.Join(repoRoot(t), "themes", "simple")
	config.C.Blog.Domain = "https://example.com"

	return tmp, func() {
		config.C.Data = prevData
		config.C.Auth = prevAuth
		config.C.Blog = prevBlog
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	// the test binary runs from the package dir; walk up to the repo root.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for d := wd; d != "/" && d != ""; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	t.Fatal("could not locate repo root")
	return ""
}

func TestSnippetCreatePathRequiresAuth(t *testing.T) {
	_, cleanup := setupSnippetTest(t)
	defer cleanup()

	form := url.Values{"body": {"hello"}}
	req := httptest.NewRequest("POST", "/snippet", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	snippetHandler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("anon POST got %d, want 401", w.Code)
	}
}

func TestSnippetCreateAndReadRoundTrip(t *testing.T) {
	dir, cleanup := setupSnippetTest(t)
	defer cleanup()

	body := "```go\nfmt.Println(\"hi\")\n```"
	form := url.Values{"body": {body}}
	req := httptest.NewRequest("POST", "/snippet", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.SetBasicAuth("alice", "pw")
	w := httptest.NewRecorder()
	snippetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("auth POST got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"url":"/snippet/`) {
		t.Errorf("expected JSON with snippet URL, got: %s", w.Body.String())
	}

	// Verify the file ended up under <data>/snippets/<id>.md.
	entries, err := os.ReadDir(filepath.Join(dir, "snippets"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 snippet file, got %d", len(entries))
	}
	name := entries[0].Name()
	id := strings.TrimSuffix(name, ".md")

	// GET /snippet/<id> should render through the article pipeline.
	req2 := httptest.NewRequest("GET", "/snippet/"+id, nil)
	w2 := httptest.NewRecorder()
	snippetHandler(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("GET snippet got %d, body=%s", w2.Code, w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "fmt.Println") {
		t.Errorf("rendered snippet missing body: %s", w2.Body.String())
	}
	if !strings.Contains(w2.Body.String(), "<pre><code") {
		t.Errorf("rendered snippet missing fenced code: %s", w2.Body.String())
	}
}

func TestSnippetRejectsBadID(t *testing.T) {
	_, cleanup := setupSnippetTest(t)
	defer cleanup()

	for _, bad := range []string{"abc", "..%2fetc%2fpasswd", "with%20space", strings.Repeat("a", 100)} {
		req := httptest.NewRequest("GET", "/snippet/"+bad, nil)
		w := httptest.NewRecorder()
		snippetHandler(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("id %q: got %d, want 404", bad, w.Code)
		}
	}
}

func TestSnippetGetForm(t *testing.T) {
	_, cleanup := setupSnippetTest(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/snippet", nil)
	w := httptest.NewRecorder()
	snippetHandler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /snippet form got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<form") {
		t.Errorf("expected an HTML form, got: %s", w.Body.String())
	}
}
