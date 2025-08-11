package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

  "github.com/gorilla/mux"
	"github.com/tinoosan/torrus/internal/handlers"
)

func main() {

	l := log.New(os.Stdout, "torrus-api ", log.LstdFlags)

	// create the handlers
	downloadHandler := handlers.NewDownloads(l)

	// create a new serve mux and register the handlers
	r := mux.NewRouter()

	getRouter := r.Methods("GET").Subrouter()
	getRouter.HandleFunc("/downloads", downloadHandler.GetDownloads)
	getRouter.HandleFunc("/downloads/{id:[0-9]+}", downloadHandler.GetDownload)

	postRouter := r.Methods("POST").Subrouter()
	postRouter.HandleFunc("/downloads", downloadHandler.AddDownload)

	patchRouter := r.Methods("PATCH").Subrouter()
	patchRouter.HandleFunc("/downloads/{id:[0-9]+}", downloadHandler.UpdateDownload)

	server := &http.Server{
		Addr:         ":9090",
		Handler:      r,
		IdleTimeout:  120 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
	}

	go func() {
		log.Println("Starting Torrus API on", server.Addr)
		if err := server.ListenAndServe(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
  signal.Notify(sigChan, os.Kill)

	sig  := <- sigChan
	l.Println("Received terminate, graceful shutdown", sig)

	timeout := 30 * time.Second
	timeoutContext, _ := context.WithTimeout(context.Background(), timeout)
	server.Shutdown(timeoutContext)

}
