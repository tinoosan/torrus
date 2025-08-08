package handlers

import (
	"log"
	"net/http"

	"github.com/tinoosan/torrus/internal/data"
)

type Downloads struct {
	l *log.Logger
}

func NewDownloads(l *log.Logger) *Downloads {
	return &Downloads{l}
}

func (d *Downloads) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	 if r.Method == http.MethodGet {
		d.getDownloads(w, r)
		return
	}
	 if r.Method == http.MethodPost {
    d.addDownload(w, r)
		return
	}

	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (d *Downloads) getDownloads(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Handle GET Download")
	dl := data.GetDownloads()
	err := dl.ToJSON(w)
	if err != nil {
		http.Error(w, "Unable to marshal json", http.StatusInternalServerError)
	}
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
}
