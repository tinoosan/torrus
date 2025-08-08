package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
)

type Download struct {
	l *log.Logger
}

func NewDownload(l *log.Logger) *Download {
	return &Download{l}
}

func (d *Download) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	d.l.Println("Downloading files...")
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "oops", http.StatusBadRequest)
		return
	}
	d.l.Printf("Data %s\n", data)

	fmt.Fprintf(w, "%s", data)
}
