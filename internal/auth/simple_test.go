package auth

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestMiddleware(t *testing.T) {
    t.Run("allows healthz without token", func(t *testing.T) {
        t.Setenv("TORRUS_API_TOKEN", "sekrit")
        handled := false
        next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            handled = true
            w.WriteHeader(http.StatusTeapot)
        })
        req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
        rr := httptest.NewRecorder()
        Middleware(next).ServeHTTP(rr, req)
        if rr.Code != http.StatusTeapot {
            t.Fatalf("expected status %d got %d", http.StatusTeapot, rr.Code)
        }
        if !handled {
            t.Fatalf("next handler not called")
        }
    })

    t.Run("rejects missing token", func(t *testing.T) {
        t.Setenv("TORRUS_API_TOKEN", "sekrit")
        handled := false
        next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            handled = true
        })
        req := httptest.NewRequest(http.MethodGet, "/", nil)
        rr := httptest.NewRecorder()
        Middleware(next).ServeHTTP(rr, req)
        if rr.Code != http.StatusUnauthorized {
            t.Fatalf("expected status %d got %d", http.StatusUnauthorized, rr.Code)
        }
        if handled {
            t.Fatalf("next handler should not be called")
        }
        if strings.TrimSpace(rr.Body.String()) != "missing API token" {
            t.Fatalf("unexpected body %q", rr.Body.String())
        }
    })

    t.Run("rejects invalid token", func(t *testing.T) {
        t.Setenv("TORRUS_API_TOKEN", "sekrit")
        handled := false
        next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            handled = true
        })
        req := httptest.NewRequest(http.MethodGet, "/", nil)
        req.Header.Set("Authorization", "Bearer wrong")
        rr := httptest.NewRecorder()
        Middleware(next).ServeHTTP(rr, req)
        if rr.Code != http.StatusForbidden {
            t.Fatalf("expected status %d got %d", http.StatusForbidden, rr.Code)
        }
        if handled {
            t.Fatalf("next handler should not be called")
        }
        if strings.TrimSpace(rr.Body.String()) != "invalid API token" {
            t.Fatalf("unexpected body %q", rr.Body.String())
        }
    })

    t.Run("allows valid token", func(t *testing.T) {
        t.Setenv("TORRUS_API_TOKEN", "sekrit")
        handled := false
        next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            handled = true
            w.WriteHeader(http.StatusCreated)
        })
        req := httptest.NewRequest(http.MethodGet, "/", nil)
        req.Header.Set("Authorization", "Bearer sekrit")
        rr := httptest.NewRecorder()
        Middleware(next).ServeHTTP(rr, req)
        if rr.Code != http.StatusCreated {
            t.Fatalf("expected status %d got %d", http.StatusCreated, rr.Code)
        }
        if !handled {
            t.Fatalf("next handler not called")
        }
    })
}

