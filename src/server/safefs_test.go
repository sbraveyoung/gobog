package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestSafeServeFileRejectsTraversal sanity-checks the safeServeFile helper:
// it must serve files under <root>/<prefix> and refuse anything that escapes
// either the prefix or the root via "..".
func TestSafeServeFileRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "image"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image", "ok.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Outside the /image/ scope but inside <root>: must NOT be served.
	if err := os.WriteFile(filepath.Join(root, "secret.toml"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		urlPath  string
		wantCode int
	}{
		{"/image/ok.txt", http.StatusOK},
		{"/image/missing.txt", http.StatusNotFound},
		{"/image/../secret.toml", http.StatusNotFound},
		{"/image/../../etc/passwd", http.StatusNotFound},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", c.urlPath, nil)
		w := httptest.NewRecorder()
		safeServeFile(w, req, root, "/image/")
		if w.Code != c.wantCode {
			t.Errorf("%s: got %d, want %d (body=%q)", c.urlPath, w.Code, c.wantCode, w.Body.String())
		}
	}
}
