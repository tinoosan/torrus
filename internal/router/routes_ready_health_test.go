package router

import (
    "context"
    "errors"
    "log/slog"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/downloader"
)

// fakeDownloadSvc is a stub to satisfy service.Download in router tests.
type fakeDownloadSvc struct{}

func (f *fakeDownloadSvc) List(ctx context.Context) (data.Downloads, error) { return nil, nil }
func (f *fakeDownloadSvc) Get(ctx context.Context, id string) (*data.Download, error) { return nil, data.ErrNotFound }
func (f *fakeDownloadSvc) Add(ctx context.Context, d *data.Download) (*data.Download, bool, error) {
    return nil, false, nil
}
func (f *fakeDownloadSvc) UpdateDesiredStatus(ctx context.Context, id string, status data.DownloadStatus) (*data.Download, error) {
    return nil, nil
}
func (f *fakeDownloadSvc) Delete(ctx context.Context, id string, deleteFiles bool) error { return nil }

// fakeDownloader allows toggling Ping behaviour.
type fakeDownloader struct{ pingErr error }

func (f *fakeDownloader) Start(context.Context, *data.Download) (string, error) { return "", nil }
func (f *fakeDownloader) Pause(context.Context, *data.Download) error { return nil }
func (f *fakeDownloader) Resume(context.Context, *data.Download) error { return nil }
func (f *fakeDownloader) Cancel(context.Context, *data.Download) error { return nil }
func (f *fakeDownloader) Delete(context.Context, *data.Download, bool) error { return nil }
func (f *fakeDownloader) Ping(ctx context.Context) error { return f.pingErr }

var _ downloader.Downloader = (*fakeDownloader)(nil)

func TestHealthzOK(t *testing.T) {
    r := New(slog.Default(), &fakeDownloadSvc{}, &fakeDownloader{})

    req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", w.Code)
    }
    if got := w.Body.String(); got != "ok" {
        t.Fatalf("expected body 'ok', got %q", got)
    }
}

func TestReadyzSuccess(t *testing.T) {
    r := New(slog.Default(), &fakeDownloadSvc{}, &fakeDownloader{pingErr: nil})
    req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != http.StatusOK {
        t.Fatalf("expected 200, got %d", w.Code)
    }
}

func TestReadyzFailure(t *testing.T) {
    r := New(slog.Default(), &fakeDownloadSvc{}, &fakeDownloader{pingErr: errors.New("nope")})
    req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    if w.Code != http.StatusServiceUnavailable {
        t.Fatalf("expected 503, got %d", w.Code)
    }
}
