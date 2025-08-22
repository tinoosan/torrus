package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/gorilla/mux"
	"github.com/tinoosan/torrus/internal/handlers"
)

type LogOptions struct {
	Format, Path          string
	MaxSize, Backups, Age int
	Logger                *lumberjack.Logger
}

func (l *LogOptions) configLogOptions() (*lumberjack.Logger, error) {
	l.Format = os.Getenv("LOG_FORMAT")
	l.Path = os.Getenv("LOG_FILE_PATH")
	if l.Path == "" {
		l.Path = "./logs/torrus.log"
	}

	err := os.MkdirAll(filepath.Dir(l.Path), 0o755)
	if err != nil {
		return nil, fmt.Errorf("make log dir: %w", err)
	}

	l.MaxSize = intFromEnv("LOG_MAX_SIZE", 1)
	l.Backups = intFromEnv("LOG_MAX_BACKUPS", 3)
	l.Age = intFromEnv("LOG_MAX_AGE_DAYS", 7)

	return &lumberjack.Logger{
		Filename:   l.Path,
		MaxSize:    l.MaxSize, // megabytes
		MaxBackups: l.Backups,
		MaxAge:     l.Age, // days
		Compress:   false,
	}, nil

}

func intFromEnv(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return def
}

func main() {

	var logger *slog.Logger

	logOptions := &LogOptions{}

	rotator, err := logOptions.configLogOptions()
	if err != nil {
		fmt.Printf("Error: %v", err)
		return
	}
	defer rotator.Close()

	multiOut := io.MultiWriter(os.Stdout, rotator)

	switch strings.ToLower(logOptions.Format) {
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
			logger.Error("Server error:", "err", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received terminate, graceful shutdown", "signal", sig)
	timeout := 30 * time.Second
	timeoutContext, _ := context.WithTimeout(context.Background(), timeout)
	if err := server.Shutdown(timeoutContext); err != nil {
		logger.Error("Graceful shutdown failed", "err", err)
	}

}
