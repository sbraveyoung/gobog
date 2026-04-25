package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"

	"github.com/SmartBrave/gobog/src/config"
)

// authConfigured reports whether [auth] in the config is fully populated.
// Without both a username and a password hash there's no credential to check
// against, so any auth-gated endpoint should fail closed.
func authConfigured() bool {
	return config.C.Auth.Username != "" && config.C.Auth.PasswordHash != ""
}

// requireAuth returns true when the request carries valid HTTP Basic
// credentials matching [auth].username / sha256([auth].password). When it
// returns false it has already written the 401/503 response — the caller
// must just return.
//
// Comparison uses constant-time equality on both the username and the
// hashed password to avoid trivial timing oracles.
func requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if !authConfigured() {
		http.Error(w, "auth not configured: set [auth].username + [auth].password_hash", http.StatusServiceUnavailable)
		return false
	}
	user, pass, ok := r.BasicAuth()
	if ok && constantEqual(user, config.C.Auth.Username) &&
		constantEqual(hashPassword(pass), config.C.Auth.PasswordHash) {
		return true
	}
	realm := config.C.Auth.Realm
	if realm == "" {
		realm = "gobog"
	}
	w.Header().Set("WWW-Authenticate", `Basic realm="`+realm+`"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
	return false
}

// hashPassword returns the lowercase hex sha256 of the given password.
// Public so admin tooling / tests can produce hashes without copying the
// implementation.
func hashPassword(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

func constantEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
