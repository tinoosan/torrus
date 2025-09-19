package v1

import (
    "context"
    "errors"
    "net/http"
    "time"

    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/reqid"
)

func MiddlewareDownloadValidation(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Decode with strict fields and consistent validation
        dl := &data.Download{}
        if err := decodeJSONStrict(w, r, dl, 1<<20, "application/json"); err != nil {
            markErr(w, err)
            if errors.Is(err, ErrContentType) {
                http.Error(w, ErrContentType.Error(), http.StatusUnsupportedMediaType)
            } else {
                http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
            }
            return
        }

		// Enforce read-only fields: reject if client sets name.
		if dl.Name != "" {
			markErr(w, ErrReadOnlyName)
			http.Error(w, ErrReadOnlyName.Error(), http.StatusBadRequest)
			return
		}
        // Enforce read-only fields: reject if client sets files.
        if len(dl.Files) > 0 {
            markErr(w, ErrReadOnlyFiles)
            http.Error(w, ErrReadOnlyFiles.Error(), http.StatusBadRequest)
            return
        }

		ctx := context.WithValue(r.Context(), ctxKeyDownload{}, dl)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func MiddlewarePatchDesired(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        var body patchBody
        if err := decodeJSONStrict(w, r, &body, 1<<20, "application/json"); err != nil {
            markErr(w, err)
            if errors.Is(err, ErrContentType) {
                http.Error(w, ErrContentType.Error(), http.StatusUnsupportedMediaType)
            } else {
                // Preserve original behavior: do not prefix with "invalid JSON: "
                http.Error(w, err.Error(), http.StatusBadRequest)
            }
            return
        }

		if body.DesiredStatus == "" {
			markErr(w, ErrDesiredStatusJSON)
			http.Error(w, ErrDesiredStatusJSON.Error(), http.StatusBadRequest)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyPatch{}, body)
		next.ServeHTTP(w, r.WithContext(ctx))

	})
}

func (dh *DownloadHandler) Log(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        startTime := time.Now()
        rw := &rwLogger{ResponseWriter: w}
        next.ServeHTTP(rw, r)
        if rw.status == 0 {
            rw.status = http.StatusOK
        }
        timeElapsed := time.Since(startTime)
        hErr := rw.err
        // Enrich logger with request_id when available
        log := dh.l
        if id, ok := reqid.From(r.Context()); ok {
            log = log.With("request_id", id)
        }
        if hErr != nil {
            log.Error(hErr.Error(),
                "method", r.Method,
                "url", r.URL.Path,
                "status", rw.status,
                "remote", r.RemoteAddr,
                "ua", r.UserAgent(),
                "dur_ms", timeElapsed.Milliseconds(),
                "bytes", rw.bytes)
            return
        }

        log.Info("", "method", r.Method,
            "url", r.URL.Path,
            "status", rw.status,
            "remote", r.RemoteAddr,
            "ua", r.UserAgent(),
            "dur_ms", timeElapsed.Milliseconds(),
            "bytes", rw.bytes)
    })
}
