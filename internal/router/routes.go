package router

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	v1 "github.com/tinoosan/torrus/api/v1"
	"github.com/tinoosan/torrus/internal/auth"
	"github.com/tinoosan/torrus/internal/service"
)

// New sets up the application routes and required middleware.
func New(logger *slog.Logger, downloadSvc service.Download) *mux.Router {

	r := mux.NewRouter()
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			logger.Error("write healthz response", "err", err)
		}
	}).Methods("GET")

	downloadHandler := v1.NewDownloadHandler(logger, downloadSvc)

	r.Use(downloadHandler.Log)
	r.Use(auth.Middleware)

	api := r.PathPrefix("/v1").Subrouter()

	// GETs
	get := api.Methods("GET").Subrouter()
	get.HandleFunc("/downloads", downloadHandler.GetDownloads)
	get.HandleFunc("/downloads/{id:[0-9]+}", downloadHandler.GetDownload)

	// POSTs
	post := api.Methods("POST").Subrouter()
	post.HandleFunc("/downloads", downloadHandler.AddDownload)
	post.Use(v1.MiddlewareDownloadValidation)

	// PATCHes
	patch := api.Methods("PATCH").Subrouter()
	patch.HandleFunc("/downloads/{id:[0-9]+}", downloadHandler.UpdateDownload)
	patch.Use(v1.MiddlewarePatchDesired)

	return r
}
