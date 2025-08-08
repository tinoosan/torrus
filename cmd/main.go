package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/tinoosan/torrus/internal/handlers"
)

func main() {

	l := log.New(os.Stdout, "torrus-api ", log.LstdFlags)
	downloadHandler := handlers.NewDownload(l)

	serveMux := http.NewServeMux()
	serveMux.Handle("/downloads", downloadHandler)

	server := &http.Server{
		Addr:         ":9090",
		Handler:      serveMux,
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
