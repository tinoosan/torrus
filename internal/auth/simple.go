package auth

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

// Middleware returns a handler that verifies requests include the
// TORRUS_API_TOKEN value in the Authorization header.
//
// The token is compared using constant time comparison to help avoid timing
// attacks. Requests to the health check endpoint are allowed through without
// authentication.
func Middleware(next http.Handler) http.Handler {
	token := os.Getenv("TORRUS_API_TOKEN")

    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/metrics" {
            next.ServeHTTP(w, r)
            return
        }

		// Expect: Authorization: Bearer <token>
		authz := r.Header.Get("Authorization")
		if !strings.HasPrefix(authz, "Bearer ") {
			http.Error(w, "missing API token", http.StatusUnauthorized)
			return
		}

		got := strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
		if token == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "invalid API token", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
