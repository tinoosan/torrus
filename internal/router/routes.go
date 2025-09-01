package router

import (
    "context"
	"log/slog"
	"net/http"
    "time"

	"github.com/gorilla/mux"
	v1 "github.com/tinoosan/torrus/api/v1"
	"github.com/tinoosan/torrus/internal/auth"
    "github.com/tinoosan/torrus/internal/downloader"
    "github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tinoosan/torrus/internal/service"
)

// New sets up the application routes and required middleware.
func New(logger *slog.Logger, downloadSvc service.Download, dlr downloader.Downloader) *mux.Router {

    r := mux.NewRouter()
    // Request ID must be first so all downstream middleware/handlers see it
    r.Use(v1.RequestID)
	r.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("ok"))
		if err != nil {
			logger.Error("write healthz response", "err", err)
		}
	}).Methods("GET")

    // Readiness probe: try a fast Ping() when supported
    r.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
        type pinger interface{ Ping(context.Context) error }
        ctx, cancel := context.WithTimeout(r.Context(), 300*time.Millisecond)
        defer cancel()

        ready := true
        var errStr string
        if p, ok := dlr.(pinger); ok {
            if err := p.Ping(ctx); err != nil {
                ready = false
                errStr = err.Error()
            }
        } else {
            // No ping capability; consider ready
            ready = true
        }

        w.Header().Set("Content-Type", "application/json")
        if !ready {
            w.WriteHeader(http.StatusServiceUnavailable)
        }
        _, _ = w.Write([]byte(`{"ready":` + map[bool]string{true: "true", false: "false"}[ready] + func() string { if errStr != "" { return ",\"error\":\"" + errStr + "\"" } ; return "" }() + "}"))
    }).Methods("GET")

    // Prometheus metrics endpoint
    r.Handle("/metrics", promhttp.Handler()).Methods("GET")

	downloadHandler := v1.NewDownloadHandler(logger, downloadSvc)

    r.Use(downloadHandler.Log)
    r.Use(auth.Middleware)

	api := r.PathPrefix("/v1").Subrouter()

	// GETs
	get := api.Methods("GET").Subrouter()
	get.HandleFunc("/downloads", downloadHandler.GetDownloads)
	get.HandleFunc("/downloads/{id}", downloadHandler.GetDownload)

	// POSTs
	post := api.Methods("POST").Subrouter()
	post.HandleFunc("/downloads", downloadHandler.AddDownload)
	post.Use(v1.MiddlewareDownloadValidation)

	// PATCHes
	patch := api.Methods("PATCH").Subrouter()
	patch.HandleFunc("/downloads/{id}", downloadHandler.UpdateDownload)
	patch.Use(v1.MiddlewarePatchDesired)

	// DELETEs
	del := api.Methods("DELETE").Subrouter()
	del.HandleFunc("/downloads/{id}", downloadHandler.DeleteDownload)

	return r
}
