package downloader

import (
	"context"
	"errors"

	"github.com/tinoosan/torrus/internal/data"
)

// ErrNotFound is returned when the downloader cannot locate a download by ID.
var ErrNotFound = errors.New("downloader not found")

// Downloader defines the operations required to manage a download's lifecycle.
type Downloader interface {
    Start(ctx context.Context, d *data.Download) (string, error)
    Pause(ctx context.Context, d *data.Download) error
    Resume(ctx context.Context, d *data.Download) error
    Cancel(ctx context.Context, d *data.Download) error
    // Delete cancels/stops the task and, if deleteFiles is true, removes
    // payload files and any related control files from disk.
    Delete(ctx context.Context, d *data.Download, deleteFiles bool) error
}

// FileLister is implemented by downloaders that can list file paths for a
// download by its backend identifier (GID). Paths should be absolute.
type FileLister interface {
    GetFiles(ctx context.Context, gid string) ([]string, error)
}

// EventSource is implemented by downloaders that emit asynchronous events.
// Reconciler wiring can launch Run(ctx) when available to process notifications.
type EventSource interface {
	Run(ctx context.Context)
}
