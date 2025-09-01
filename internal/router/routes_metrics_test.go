package router

import (
    "io"
    "log/slog"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/tinoosan/torrus/internal/metrics"
)

func TestMetricsEndpointEmitsFamilies(t *testing.T) {
    // Register collectors and prime a couple of samples
    metrics.Register()
    metrics.DownloadEvents.WithLabelValues("start").Inc()
    metrics.Aria2RPCLatency.WithLabelValues("aria2.tellStatus").Observe(0.02)
    metrics.ActiveDownloads.Set(2)

    r := New(slog.New(slog.NewTextHandler(io.Discard, nil)), &fakeDownloadSvc{}, &fakeDownloader{})

    req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", w.Code)
    }
    body := w.Body.String()
    if !strings.Contains(body, "torrus_download_events_total") {
        t.Fatalf("missing download_events_total in metrics: %s", body)
    }
    if !strings.Contains(body, "torrus_aria2_rpc_latency_seconds_count") {
        t.Fatalf("missing aria2 latency histogram in metrics: %s", body)
    }
    if !strings.Contains(body, "torrus_active_downloads") {
        t.Fatalf("missing active_downloads gauge in metrics: %s", body)
    }
}

