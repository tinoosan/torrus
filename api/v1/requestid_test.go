package v1

import (
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/tinoosan/torrus/internal/reqid"
)

func TestRequestIDMiddleware_GeneratesAndEchoes(t *testing.T) {
    h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    h.ServeHTTP(rr, req)
    got := rr.Header().Get(headerRequestID)
    if got == "" {
        t.Fatalf("expected non-empty %s header", headerRequestID)
    }
}

func TestRequestIDMiddleware_HonorsIncoming(t *testing.T) {
    h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    req.Header.Set(headerRequestID, "abc123")
    h.ServeHTTP(rr, req)
    if rr.Header().Get(headerRequestID) != "abc123" {
        t.Fatalf("expected echoed header abc123, got %q", rr.Header().Get(headerRequestID))
    }
}

// stub service implementing service.Download and recording the context.
// no-op: previous testDownloadSvc removed; tests now assert middleware behavior directly

// Smoke test: ensure middleware injects header and context seen by handler/service.
func TestRequestID_PropagatesIntoHandlerContext(t *testing.T) {
    // Compose middleware with a tiny handler that observes the request_id from context
    observedHeader := "X-Observed-Request-ID"
    h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if id, ok := reqid.From(r.Context()); ok {
            w.Header().Set(observedHeader, id)
        }
        w.WriteHeader(http.StatusOK)
    }))
    rr := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/", nil)
    req.Header.Set(headerRequestID, "abc123")
    h.ServeHTTP(rr, req)
    if rr.Header().Get(headerRequestID) != "abc123" {
        t.Fatalf("expected echoed X-Request-ID header")
    }
    if rr.Header().Get(observedHeader) != "abc123" {
        t.Fatalf("handler did not observe request_id in context; got %q", rr.Header().Get(observedHeader))
    }
}
