package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/tinoosan/torrus/internal/handlers"
)

func main() {

	var logger *slog.Logger

	f, err := os.OpenFile("./torrus.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		fmt.Println("log file could not be opened or created: ", err)
		return
	}

	multiOut := io.MultiWriter(f, os.Stdout)

	logFormat := os.Getenv("LOG_FORMAT")

	switch strings.ToLower(logFormat) {
	case "json":
		logger = slog.New(slog.NewJSONHandler(multiOut, nil))
	default:
		logger = slog.New(slog.NewTextHandler(multiOut, nil))
	}

	// create the handlers
	downloadHandler := handlers.NewDownloads(logger)

	// create a new serve mux and register the handlers
	r := mux.NewRouter()

	r.Use(downloadHandler.Log)

	getRouter := r.Methods("GET").Subrouter()
	getRouter.HandleFunc("/downloads", downloadHandler.GetDownloads)
	getRouter.HandleFunc("/downloads/{id:[0-9]+}", downloadHandler.GetDownload)

	postRouter := r.Methods("POST").Subrouter()
	postRouter.HandleFunc("/downloads", downloadHandler.AddDownload)
	postRouter.Use(downloadHandler.MiddlewareDownloadValidation)

	patchRouter := r.Methods("PATCH").Subrouter()
	patchRouter.HandleFunc("/downloads/{id:[0-9]+}", downloadHandler.UpdateDownload)
	patchRouter.Use(downloadHandler.MiddlewarePatchDesired)

	server := &http.Server{
		Addr:         ":9090",
		Handler:      r,
		IdleTimeout:  120 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
	}


	go func() {
		logger.Info("Starting Torrus API on", "port", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			logger.Error("Server error:", "err", err.Error())
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received terminate, graceful shutdown", "signal", sig)
	defer f.Close()

	timeout := 30 * time.Second
	timeoutContext, _ := context.WithTimeout(context.Background(), timeout)
	if err := server.Shutdown(timeoutContext); err != nil {
  	logger.Error("Graceful shutdown failed", "err", err)
	}

}
