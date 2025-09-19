package v1

import (
    "encoding/json"
    "errors"
    "net/http"
    "strings"
)

// decodeJSONStrict validates optional Content-Type, enforces a max body size,
// and decodes JSON into dst while disallowing unknown fields. It returns
// ErrContentType when the Content-Type header is present but not acceptable.
func decodeJSONStrict(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64, contentTypePrefix string) error {
    if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, contentTypePrefix) {
        return ErrContentType
    }
    // Limit body to prevent abuse.
    r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(dst); err != nil {
        // Preserve sentinel for callers to branch on content-type errors.
        if errors.Is(err, ErrContentType) {
            return ErrContentType
        }
        return err
    }
    return nil
}

