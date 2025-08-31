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
	Get(ctx context.Context, id int) (*data.Download, error)
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
	Update(ctx context.Context, id int, mutate func(*data.Download) error) (*data.Download, error)
}
