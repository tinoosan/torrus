package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tinoosan/torrus/internal/data"
)

func MiddlewareDownloadValidation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, "application/json") {
			// Content type
			markErr(w, ErrContentType)
			http.Error(w, ErrContentType.Error(), http.StatusUnsupportedMediaType)
			return
		}

		// Body limit
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		// Decode with strict fields
		dl := &data.Download{}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		err := dec.Decode(dl)
		if err != nil {
			markErr(w, err)
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
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
		if contentType := r.Header.Get("Content-Type"); contentType != "" && !strings.HasPrefix(contentType, "application/json") {
			markErr(w, ErrContentType)
			http.Error(w, ErrContentType.Error(), http.StatusUnsupportedMediaType)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

		var body patchBody
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		err := dec.Decode(&body)
		if err != nil {
			markErr(w, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
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
		if hErr != nil {
			dh.l.Error(hErr.Error(),
				"method", r.Method,
				"url", r.URL.Path,
				"status", rw.status,
				"remote", r.RemoteAddr,
				"ua", r.UserAgent(),
				"dur_ms", timeElapsed.Milliseconds(),
				"bytes", rw.bytes)
			return
		}

		dh.l.Info("", "method", r.Method,
			"url", r.URL.Path,
			"status", rw.status,
			"remote", r.RemoteAddr,
			"ua", r.UserAgent(),
			"dur_ms", timeElapsed.Milliseconds(),
			"bytes", rw.bytes)
	})
}
