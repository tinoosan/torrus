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

	lumberjack "gopkg.in/natefinch/lumberjack.v2"

	"github.com/tinoosan/torrus/internal/aria2"
	"github.com/tinoosan/torrus/internal/downloader"
	aria2dl "github.com/tinoosan/torrus/internal/downloader/aria2"
	"github.com/tinoosan/torrus/internal/metrics"
	"github.com/tinoosan/torrus/internal/reconciler"
	"github.com/tinoosan/torrus/internal/repo"
	"github.com/tinoosan/torrus/internal/router"
	"github.com/tinoosan/torrus/internal/service"
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
	defer func() {
		err := rotator.Close()
		if err != nil {
			fmt.Printf("close log file: %v", err)
		}
	}()

	multiOut := io.MultiWriter(os.Stdout, rotator)

	switch strings.ToLower(logOptions.Format) {
	case "json":
		logger = slog.New(slog.NewJSONHandler(multiOut, nil))
	default:
		logger = slog.New(slog.NewTextHandler(multiOut, nil))
	}

	downloadRepo := repo.NewInMemoryDownloadRepo()
	events := make(chan downloader.Event, 16)
	rep := downloader.NewChanReporter(events)

	var dlr downloader.Downloader
	switch os.Getenv("TORRUS_CLIENT") {
	case "aria2":
        aria2Client, err := aria2.NewClientFromEnv()
        if err != nil {
            fmt.Println("Error:", err)
            dlr = downloader.NewNoopDownloader()
        } else {
            a := aria2dl.NewAdapter(aria2Client, rep)
            a.SetLogger(logger)
            dlr = a
        }
	default:
		dlr = downloader.NewNoopDownloader()
	}

    downloadSvc := service.NewDownload(downloadRepo, dlr)

	// Register Prometheus metrics collectors
	metrics.Register()

	rec := reconciler.New(logger, downloadRepo, events)
	rec.Run()

	// If the downloader emits events, launch its event loop.
	if src, ok := dlr.(downloader.EventSource); ok {
		go src.Run(context.Background())
	}

	r := router.New(logger, downloadSvc, dlr)

	server := &http.Server{
		Addr:         ":9090",
		Handler:      r,
		IdleTimeout:  120 * time.Second,
		ReadTimeout:  1 * time.Second,
		WriteTimeout: 1 * time.Second,
	}

	go func() {
		logger.Info("logging configured",
			"format", strings.ToLower(logOptions.Format),
			"file", logOptions.Path,
			"rotate_mb", logOptions.MaxSize,
			"rotate_backups", logOptions.Backups,
			"rotate_age_days", logOptions.Age,
		)
		logger.Info("Starting Torrus API on", "port", server.Addr)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			logger.Error("Server error:", "err", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	sig := <-sigChan
	logger.Info("Received terminate, graceful shutdown", "signal", sig)
	timeout := 30 * time.Second
	timeoutContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err = server.Shutdown(timeoutContext)
	if err != nil {
		logger.Error("Graceful shutdown failed", "err", err)
	}
	rec.Stop()

}
