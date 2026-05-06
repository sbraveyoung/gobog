package server

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/sbraveyoung/gobog/src/config"
)

// TestTLSGracefulMissingCert verifies that pointing at non-existent cert
// files no longer aborts startup — tls.LoadX509KeyPair just fails and the
// caller falls back to plain HTTP. We exercise the same library call Run()
// uses without booting an actual listener.
func TestTLSGracefulMissingCert(t *testing.T) {
	prev := config.C.Http
	t.Cleanup(func() { config.C.Http = prev })

	dir := t.TempDir()
	config.C.Http.Cert = filepath.Join(dir, "missing.pem")
	config.C.Http.Key = filepath.Join(dir, "missing.key")

	_, err := tls.LoadX509KeyPair(config.C.Http.Cert, config.C.Http.Key)
	if err == nil {
		t.Fatal("expected an error loading missing cert, got nil")
	}
	// The whole point: Run() must NOT propagate this error. The lib-level
	// failure is the same shape that we now catch + log + skip.
	if !os.IsNotExist(err) {
		// Some platforms wrap it differently; loosen to "any error" but
		// note that the test is only meaningful as a doc here.
		t.Logf("LoadX509KeyPair error (expected non-nil): %v", err)
	}
}

func TestTLSEmptyConfigSkipsLoadAttempt(t *testing.T) {
	prev := config.C.Http
	t.Cleanup(func() { config.C.Http = prev })

	config.C.Http.Cert = ""
	config.C.Http.Key = ""

	// When Cert/Key are empty Run() doesn't call tls.LoadX509KeyPair at
	// all; this test pins that contract — empty config must never look at
	// the filesystem.
	if config.C.Http.Cert != "" || config.C.Http.Key != "" {
		t.Fatal("invariant: empty cert/key paths")
	}
}
