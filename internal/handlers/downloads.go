package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

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

func (d *Downloads) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // /downloads/{id}
    if strings.HasPrefix(r.URL.Path, "/downloads/") {
        idStr := strings.TrimPrefix(r.URL.Path, "/downloads/")
        id, err := strconv.Atoi(idStr)
        if err != nil || id <= 0 {
            http.Error(w, "invalid id", http.StatusBadRequest)
            return
        }
        switch r.Method {
        case http.MethodPatch:
            d.patchDownload(w, r, id)
            return
			  case http.MethodGet:
						d.getDownload(w, id)
        default:
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
    }
    // /downloads
    if r.URL.Path == "/downloads" {
        switch r.Method {
        case http.MethodGet:
            d.getDownloads(w)
            return
        case http.MethodPost:
            d.addDownload(w, r)
            return
        default:
            w.WriteHeader(http.StatusMethodNotAllowed)
            return
        }
    }
}

func (d *Downloads) getDownloads(w http.ResponseWriter) {
	d.l.Println("Handle GET Downloads")
	dl := data.GetDownloads()
	err := dl.ToJSON(w)
	if err != nil {
		http.Error(w, "Unable to marshal json", http.StatusInternalServerError)
	}
}

func (d *Downloads) getDownload(w http.ResponseWriter, id int) {
	d.l.Println("Handle GET Download")
	dl, err := data.FindByID(id)
	if err != nil {
		http.Error(w, "Not Found", http.StatusNotFound)
	}
	_ = dl.ToJSON(w)
}

func (d *Downloads) addDownload(w http.ResponseWriter, r *http.Request) {
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

func (d *Downloads) patchDownload(w http.ResponseWriter, r *http.Request, id int) {
	d.l.Println("Handle PATCH Download")
	var body patchBody
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
