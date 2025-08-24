package router

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	v1 "github.com/tinoosan/torrus/api/v1"
)

func New(logger *slog.Logger) *mux.Router {

	r := mux.NewRouter()
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}).Methods("GET")

	downloadHandler := v1.NewDownloads(logger)

	r.Use(downloadHandler.Log)

	api := r.PathPrefix("/v1").Subrouter()

	// GETs
	get := api.Methods("GET").Subrouter()
	get.HandleFunc("/downloads", downloadHandler.GetDownloads)
	get.HandleFunc("/downloads/{id:[0-9]+}", downloadHandler.GetDownload)

	// POSTs
	post := api.Methods("POST").Subrouter()
	post.HandleFunc("/downloads", downloadHandler.AddDownload)
	post.Use(downloadHandler.MiddlewareDownloadValidation)

	// PATCHes
	patch := api.Methods("PATCH").Subrouter()
	patch.HandleFunc("/downloads/{id:[0-9]+}", downloadHandler.UpdateDownload)
	patch.Use(downloadHandler.MiddlewarePatchDesired)

	return r
}
