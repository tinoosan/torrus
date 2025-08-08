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
	 if r.Method == http.MethodGet{
		d.getDownloads(w, r)
		return
	}
	w.WriteHeader(http.StatusMethodNotAllowed)
}

func (d *Downloads) getDownloads(w http.ResponseWriter, r *http.Request) {
	dl := data.GetDownloads()
	err := dl.ToJSON(w)
	if err != nil {
		http.Error(w, "Unable to marshal json", http.StatusInternalServerError)
	}
}
