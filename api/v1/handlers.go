package v1

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/service"
)

type DownloadHandler struct {
	l   *slog.Logger
	svc service.Download
}

type patchBody struct {
	DesiredStatus string `json:"desiredStatus"`
}

type deleteBody struct {
	DeleteFiles bool `json:"deleteFiles"`
}

type rwLogger struct {
	http.ResponseWriter
	status int
	bytes  int
	err    error
}

func (w *rwLogger) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *rwLogger) SetErr(err error) {
	w.err = err
}

func (w *rwLogger) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}

	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

type errorSetter interface {
	SetErr(error)
}

func markErr(w http.ResponseWriter, err error) {
	if es, ok := w.(errorSetter); ok {
		es.SetErr(err)
	}
}

// context keys
type ctxKeyDownload struct{}
type ctxKeyPatch struct{}

func NewDownloadHandler(l *slog.Logger, svc service.Download) *DownloadHandler {
	return &DownloadHandler{l: l, svc: svc}
}

func (dh *DownloadHandler) GetDownloads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dl, err1 := dh.svc.List(r.Context())
	if err1 != nil {
		markErr(w, err1)
		http.Error(w, err1.Error(), http.StatusInternalServerError)
		return
	}
	err := dl.ToJSON(w)
	if err != nil {
		markErr(w, err)
		http.Error(w, "Unable to marshal json", http.StatusInternalServerError)
		return
	}
}

func (dh *DownloadHandler) GetDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	dl, err := dh.svc.Get(r.Context(), id)
	if err != nil {
		markErr(w, err)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = dl.ToJSON(w)

}

func (dh *DownloadHandler) AddDownload(w http.ResponseWriter, r *http.Request) {

	v := r.Context().Value(ctxKeyDownload{})
	dl, ok := v.(*data.Download)
	if !ok || dl == nil {
		markErr(w, ErrDownloadCtx)
		http.Error(w, ErrDownloadCtx.Error(), http.StatusInternalServerError)
		return
	}
	saved, created, err := dh.svc.Add(r.Context(), dl)
	switch {
	case errors.Is(err, data.ErrInvalidSource), errors.Is(err, data.ErrTargetPath):
		markErr(w, err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		http.Error(w, "failed to create", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = saved.ToJSON(w)
}

func (dh *DownloadHandler) UpdateDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	v := r.Context().Value(ctxKeyPatch{})
	body, ok := v.(patchBody)
	if !ok || body.DesiredStatus == "" {
		markErr(w, ErrDesiredStatus)
		http.Error(w, ErrDesiredStatus.Error(), http.StatusInternalServerError)
		return
	}

	updated, err := dh.svc.UpdateDesiredStatus(r.Context(), id, data.DownloadStatus(body.DesiredStatus))
	if err != nil {
		switch err {
		case data.ErrNotFound:
			markErr(w, err)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		case data.ErrBadStatus:
			markErr(w, err)
			http.Error(w, "Invalid desiredStatus (allowed: Active|Resume|Paused|Cancelled)", http.StatusBadRequest)
			return
		case data.ErrConflict:
			markErr(w, err)
			http.Error(w, "Conflict: target file exists", http.StatusConflict)
			return
		default:
			markErr(w, err)
			http.Error(w, "failed to update", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = updated.ToJSON(w)
}

func (dh *DownloadHandler) DeleteDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var body deleteBody
	if r.Body != nil && r.ContentLength != 0 {
		if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
			markErr(w, ErrContentType)
			http.Error(w, ErrContentType.Error(), http.StatusBadRequest)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&body); err != nil {
			markErr(w, err)
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if err := dh.svc.Delete(r.Context(), id, body.DeleteFiles); err != nil {
		switch err {
		case data.ErrNotFound:
			markErr(w, err)
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		case data.ErrConflict:
			markErr(w, err)
			http.Error(w, err.Error(), http.StatusConflict)
			return
		default:
			markErr(w, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusNoContent)
}
