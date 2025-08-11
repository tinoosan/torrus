package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/tinoosan/torrus/internal/data"
)

type Downloads struct {
	l *log.Logger
}

type patchBody struct {
	DesiredStatus string `json:"desiredStatus"`
}


// context keys
type ctxKeyDownload struct{}
type ctxKeyPatch struct{}

func NewDownloads(l *log.Logger) *Downloads {
	return &Downloads{l}
}

func (d *Downloads) GetDownloads(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle GET Downloads")
	dl := data.GetDownloads()
	err := dl.ToJSON(w)
	if err != nil {
		http.Error(w, "Unable to marshal json", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
}

func (d *Downloads) GetDownload(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle GET Download")

	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Unable to convert ID", http.StatusBadRequest)
		return
	}

	dl, err := data.FindByID(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = dl.ToJSON(w)
}

func (d *Downloads) AddDownload(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle POST Download")

	v := r.Context().Value(ctxKeyDownload{})
	dl, ok := v.(*data.Download)
	if !ok || dl == nil {
		http.Error(w, "download missing in context", http.StatusInternalServerError)
		return
	}
	dl.CreatedAt = time.Now()
	data.AddDownload(dl)
	d.l.Printf("Download: %#v", dl)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = dl.ToJSON(w)
}

func (d *Downloads) UpdateDownload(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle PATCH Download")
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Unable to convert ID", http.StatusBadRequest)
		return
	}

	v := r.Context().Value(ctxKeyPatch{})
	body, ok := v.(patchBody)
	if !ok || body.DesiredStatus == "" {
		http.Error(w, "desired status missing in context", http.StatusInternalServerError)
		return
	}

	updated, err := data.UpdateDesiredStatus(id, data.DownloadStatus(body.DesiredStatus))
	if err != nil {
		switch err {
		case data.ErrNotFound:
			http.Error(w, "Not found", http.StatusNotFound)
			return
		case data.ErrBadStatus:
			http.Error(w, "Invalid desiredStatus (allowed: Active|Paused|Cancelled)", http.StatusBadRequest)
			return
		default:
			http.Error(w, "failed to update", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	_ = updated.ToJSON(w)
}



func (d *Downloads) MiddlewareDownloadValidation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dl := &data.Download{}

		err := dl.FromJSON(r.Body)
		if err != nil {
			http.Error(w, "Error reading product", http.StatusBadRequest)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyDownload{}, dl)
		req := r.WithContext(ctx)
		next.ServeHTTP(w, req)
	})
}

func (d *Downloads) MiddlewarePatchDesired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body patchBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DesiredStatus == "" {
			http.Error(w, "missing desiredStatus", http.StatusBadRequest)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyPatch{}, body)
		next.ServeHTTP(w, r.WithContext(ctx))
		
	})
}
