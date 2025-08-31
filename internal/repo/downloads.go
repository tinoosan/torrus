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

// DownloadWriter defines write operations for downloads.
type DownloadWriter interface {
	Add(ctx context.Context, download *data.Download) (*data.Download, error)
	UpdateDesiredStatus(ctx context.Context, id int, status data.DownloadStatus) (*data.Download, error)
	SetStatus(ctx context.Context, id int, status data.DownloadStatus) error
	SetGID(ctx context.Context, id int, gid string) error
	ClearGID(ctx context.Context, id int) error
}
