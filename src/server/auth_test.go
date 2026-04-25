package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SmartBrave/gobog/src/config"
)

func TestRequireAuth(t *testing.T) {
	prev := config.C.Auth
	t.Cleanup(func() { config.C.Auth = prev })

	t.Run("not configured -> 503", func(t *testing.T) {
		config.C.Auth = config.AuthConfig{}
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		if requireAuth(w, req) {
			t.Fatal("expected requireAuth to refuse without config")
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("got %d, want 503", w.Code)
		}
	})

	t.Run("missing creds -> 401 with WWW-Authenticate", func(t *testing.T) {
		config.C.Auth = config.AuthConfig{
			Username:     "alice",
			PasswordHash: hashPassword("hunter2"),
		}
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()
		if requireAuth(w, req) {
			t.Fatal("expected requireAuth to refuse missing creds")
		}
		if w.Code != http.StatusUnauthorized {
			t.Errorf("got %d, want 401", w.Code)
		}
		if w.Header().Get("WWW-Authenticate") == "" {
			t.Error("missing WWW-Authenticate header")
		}
	})

	t.Run("wrong password -> 401", func(t *testing.T) {
		config.C.Auth = config.AuthConfig{
			Username:     "alice",
			PasswordHash: hashPassword("hunter2"),
		}
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("alice", "wrong")
		w := httptest.NewRecorder()
		if requireAuth(w, req) {
			t.Fatal("expected requireAuth to refuse wrong password")
		}
	})

	t.Run("wrong user -> 401", func(t *testing.T) {
		config.C.Auth = config.AuthConfig{
			Username:     "alice",
			PasswordHash: hashPassword("hunter2"),
		}
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("eve", "hunter2")
		w := httptest.NewRecorder()
		if requireAuth(w, req) {
			t.Fatal("expected requireAuth to refuse wrong user")
		}
	})

	t.Run("correct creds -> ok", func(t *testing.T) {
		config.C.Auth = config.AuthConfig{
			Username:     "alice",
			PasswordHash: hashPassword("hunter2"),
		}
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("alice", "hunter2")
		w := httptest.NewRecorder()
		if !requireAuth(w, req) {
			t.Fatalf("expected requireAuth to accept correct creds, got code %d", w.Code)
		}
	})
}

func TestHashPasswordIsStable(t *testing.T) {
	a := hashPassword("hunter2")
	b := hashPassword("hunter2")
	if a != b {
		t.Errorf("hash not deterministic: %s vs %s", a, b)
	}
	if a == hashPassword("hunter3") {
		t.Errorf("collision-ish: same hash for different inputs")
	}
	if len(a) != 64 {
		t.Errorf("expected hex sha256 len 64, got %d", len(a))
	}
}
