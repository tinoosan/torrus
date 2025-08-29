package v1

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/tinoosan/torrus/internal/data"
	"github.com/tinoosan/torrus/internal/repo"
)

type DownloadHandler struct {
	l *slog.Logger
	repo repo.DownloadRepo
}

type patchBody struct {
	DesiredStatus string `json:"desiredStatus"`
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

func NewDownloadHandler(l *slog.Logger, repo repo.DownloadRepo) *DownloadHandler {
	return &DownloadHandler{l: l, repo: repo}
}

func (dh *DownloadHandler) GetDownloads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dl := dh.repo.List(r.Context())
	err := dl.ToJSON(w)
	if err != nil {
		markErr(w, err)
		http.Error(w, "Unable to marshal json", http.StatusInternalServerError)
		return
	}
}

func (dh *DownloadHandler) GetDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		markErr(w, err)
		http.Error(w, "Unable to convert ID", http.StatusBadRequest)
		return
	}

	dl, err := dh.repo.Get(r.Context(), id)
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
	dh.repo.Add(r.Context(), dl)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = dl.ToJSON(w)
}

func (dh *DownloadHandler) UpdateDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		markErr(w, err)
		http.Error(w, "Unable to convert ID", http.StatusBadRequest)
		return
	}

	v := r.Context().Value(ctxKeyPatch{})
	body, ok := v.(patchBody)
	if !ok || body.DesiredStatus == "" {
		markErr(w, ErrDesiredStatus)
		http.Error(w, ErrDesiredStatus.Error(), http.StatusInternalServerError)
		return
	}

	updated, err := dh.repo.UpdateDesiredStatus(r.Context(), id, data.DownloadStatus(body.DesiredStatus))
	if err != nil {
		switch err {
		case data.ErrNotFound:
			markErr(w, err)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		case data.ErrBadStatus:
			markErr(w, err)
			http.Error(w, "Invalid desiredStatus (allowed: Active|Paused|Cancelled)", http.StatusBadRequest)
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
