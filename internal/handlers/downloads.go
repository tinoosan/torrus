package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/tinoosan/torrus/internal/data"
)

type Downloads struct {
	l *log.Logger
}

type patchBody struct {
	DesiredStatus string `json:"desiredStatus"`
}

func NewDownloads(l *log.Logger) *Downloads {
	return &Downloads{l}
}

func (d *Downloads) GetDownloads(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle GET Downloads")
	dl := data.GetDownloads()
	err := dl.ToJSON(w)
	if err != nil {
		http.Error(w, "Unable to marshal json", http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "application/json")
}

func (d *Downloads) GetDownload(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle GET Download")

	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Unable to convert ID", http.StatusBadRequest)
	}

	dl, err := data.FindByID(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = dl.ToJSON(w)
}

func (d *Downloads) AddDownload(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle POST Download")

	dl := &data.Download{}
	err := dl.FromJSON(r.Body)
	if err != nil {
		http.Error(w, "Unable to unmarshal json", http.StatusBadRequest)
	}
	data.AddDownload(dl)
	d.l.Printf("Download: %#v", dl)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = dl.ToJSON(w)
}

func (d *Downloads) UpdateDownload(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle PATCH Download")
	var body patchBody
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "Unable to convert ID", http.StatusBadRequest)
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DesiredStatus == "" {
		http.Error(w, "missing desiredStatus", http.StatusBadRequest)
		return
	}

	updated, err := data.UpdateDesiredStatus(id, data.DownloadStatus(body.DesiredStatus))
	if err != nil {
		switch err {
		case data.ErrNotFound:
			http.Error(w, "Not found", http.StatusNotFound)
		case data.ErrBadStatus:
			http.Error(w, "Invalid desiredStatus (allowed: Active|Paused|Cancelled)", http.StatusBadRequest)
		default:
			http.Error(w, "failed to update", http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Header().Set("Content-Type", "application/json")
	_ = updated.ToJSON(w)
}
