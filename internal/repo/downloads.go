package repo

import (
	"context"

	"github.com/tinoosan/torrus/internal/data"
)

// DownloadRepo aggregates the read and write capabilities for downloads.
type DownloadRepo interface {
	DownloadReader
	DownloadWriter
}

// DownloadReader defines read-only access to downloads.
type DownloadReader interface {
	List(ctx context.Context) (data.Downloads, error)
	Get(ctx context.Context, id string) (*data.Download, error)
}

// UpdateFields specifies optional updates. Nil fields mean no change.
type UpdateFields struct {
	DesiredStatus *data.DownloadStatus
	Status        *data.DownloadStatus
	// For GID: nil leaves unchanged; pointer to empty string clears it.
	GID *string
}

// DownloadWriter defines write operations for downloads.
type DownloadWriter interface {
	Add(ctx context.Context, download *data.Download) (*data.Download, error)
	Update(ctx context.Context, id string, mutate func(*data.Download) error) (*data.Download, error)
	// AddWithFingerprint performs an atomic check-then-insert based on the
	// provided fingerprint. If an existing row with the fingerprint exists,
	// it returns that row and created=false. Otherwise it inserts the provided
	// download, assigns an ID, and returns created=true.
	AddWithFingerprint(ctx context.Context, download *data.Download, fingerprint string) (*data.Download, bool, error)
	// Delete removes the download with the given ID.
	Delete(ctx context.Context, id string) error
}

// DownloadFinder extends lookup helpers
type DownloadFinder interface {
	// GetByFingerprint returns a download matched by the provided fingerprint,
	// or data.ErrNotFound if none exists.
	GetByFingerprint(ctx context.Context, fingerprint string) (*data.Download, error)
}

// ExtendedRepo combines reader, writer and finder interfaces.
type ExtendedRepo interface {
	DownloadReader
	DownloadWriter
	DownloadFinder
}
