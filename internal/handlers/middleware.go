package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/tinoosan/torrus/internal/data"
)

func (d *Downloads) MiddlewareDownloadValidation(next http.Handler) http.Handler {
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
		if err := dec.Decode(dl); err != nil {
			markErr(w, err)
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Field validation
		if !isMagnet(dl.Source) {
			markErr(w, ErrMagnetURI)
			http.Error(w, ErrMagnetURI.Error(), http.StatusBadRequest)
			return
		}

		if strings.TrimSpace(dl.TargetPath) == "" {
			markErr(w, ErrTargetPath)
			http.Error(w, ErrTargetPath.Error(), http.StatusBadRequest)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyDownload{}, dl)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (d *Downloads) MiddlewarePatchDesired(next http.Handler) http.Handler {
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
		if err := dec.Decode(&body); err != nil {
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

func (d *Downloads) Log(next http.Handler) http.Handler {
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
			d.l.Error(hErr.Error(),
				"method", r.Method,
				"url", r.URL.Path,
				"status", rw.status,
				"remote", r.RemoteAddr,
				"ua", r.UserAgent(),
				"dur_ms", timeElapsed.Milliseconds(),
				"bytes", rw.bytes)
			return
		}

		d.l.Info("", "method", r.Method,
			"url", r.URL.Path,
			"status", rw.status,
			"remote", r.RemoteAddr,
			"ua", r.UserAgent(),
			"dur_ms", timeElapsed.Milliseconds(),
			"bytes", rw.bytes)
	})
}

func isMagnet(s string) bool {
	if !strings.HasPrefix(s, "magnet:?") {
		return false
	}
	return strings.Contains(s, "xt=urn:btih:")
}
