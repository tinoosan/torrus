package auth

import (
	"crypto/subtle"
	"net/http"
	"os"
	"strings"
)

func Middleware(next http.Handler) http.Handler {
	token := os.Getenv("TORRUS_API_TOKEN")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
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
