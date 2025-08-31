package downloader

import (
    "context"
    "errors"

    "github.com/tinoosan/torrus/internal/data"
    "github.com/tinoosan/torrus/internal/downloadcfg"
)

// ErrNotFound is returned when the downloader cannot locate a download by ID.
var ErrNotFound = errors.New("downloader not found")

// Downloader defines the operations required to manage a download's lifecycle.
type Downloader interface {
    Start(ctx context.Context, d *data.Download, opts downloadcfg.StartOptions) (string, error)
    Pause(ctx context.Context, d *data.Download) error
    Resume(ctx context.Context, d *data.Download, opts downloadcfg.StartOptions) error
    Cancel(ctx context.Context, d *data.Download) error
}

// EventSource is implemented by downloaders that emit asynchronous events.
// Reconciler wiring can launch Run(ctx) when available to process notifications.
type EventSource interface {
    Run(ctx context.Context)
}
