package v1

import (
    "net/http"

    "github.com/google/uuid"
    "github.com/tinoosan/torrus/internal/reqid"
)

const headerRequestID = "X-Request-ID"

// RequestID ensures every request has a correlation ID in context and headers.
// - Honors incoming X-Request-ID if present, otherwise generates a UUIDv4.
// - Stores the value in request context via reqid.With.
// - Echoes the value in the response header.
func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := r.Header.Get(headerRequestID)
        if id == "" {
            id = uuid.NewString()
        }
        // Attach to context for downstream consumers.
        ctx := reqid.With(r.Context(), id)
        // Ensure response always includes the header.
        w.Header().Set(headerRequestID, id)
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

